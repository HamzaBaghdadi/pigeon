// Package oauth signs users in with their email provider (Google or
// Microsoft) using the OAuth 2.0 flow for native apps (RFC 8252):
//
//   - The provider's own sign-in page opens in the user's browser, so Pigeon
//     never sees the account password.
//   - The result comes back to a one-time loopback address (127.0.0.1 on a
//     random port) that only accepts a single, state-checked request.
//   - PKCE (S256) binds the code to this app instance, so an intercepted code
//     is useless.
package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// Provider describes an OAuth identity provider for email.
type Provider struct {
	Name     string // shown to users, e.g. "Google"
	AuthURL  string
	TokenURL string
	// UserInfoURL returns the account's address and name. When empty, they
	// are read from the ID token returned with the access token instead.
	UserInfoURL string
	Scopes      []string
	// Extra parameters for the authorization request.
	AuthParams map[string]string
	// RedirectHost and RedirectPath shape the loopback redirect URI and must
	// match the app registration (the port is always random). Defaults:
	// 127.0.0.1 and /callback.
	RedirectHost string
	RedirectPath string
}

func (p Provider) redirectHost() string {
	if p.RedirectHost != "" {
		return p.RedirectHost
	}
	return "127.0.0.1"
}

func (p Provider) redirectPath() string {
	if p.RedirectPath != "" {
		return p.RedirectPath
	}
	return "/callback"
}

// Google requests full IMAP/SMTP access (the only scope Gmail's IMAP accepts)
// plus the address and name of the account.
var Google = Provider{
	Name:        "Google",
	AuthURL:     "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL:    "https://oauth2.googleapis.com/token",
	UserInfoURL: "https://openidconnect.googleapis.com/v1/userinfo",
	Scopes:      []string{"https://mail.google.com/", "openid", "email", "profile"},
	// offline + consent: always return a refresh token so sync keeps working.
	AuthParams: map[string]string{"access_type": "offline", "prompt": "consent"},
}

// Microsoft covers personal accounts (Outlook.com, Hotmail, Live) and work or
// school Microsoft 365 accounts (the "common" tenant). It asks for IMAP and
// SMTP access to Exchange Online, a refresh token, and the account identity.
// The registration is a public client: no secret, PKCE only. The Azure portal
// only accepts http://localhost for loopback (the port is ignored when
// matching), so the redirect uses that host with no path.
var Microsoft = Provider{
	Name:     "Microsoft",
	AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
	TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
	Scopes: []string{
		"https://outlook.office.com/IMAP.AccessAsUser.All",
		"https://outlook.office.com/SMTP.Send",
		"offline_access", "openid", "email", "profile",
	},
	AuthParams:   map[string]string{"prompt": "select_account"},
	RedirectHost: "localhost",
	RedirectPath: "/",
}

// Client is the app's registration with a provider. For installed apps the
// "secret" is not confidential (it ships with the app); PKCE provides the security.
type Client struct {
	ID     string
	Secret string
}

// Result is what a successful sign-in returns.
type Result struct {
	Token *oauth2.Token
	Email string
	Name  string
}

// ErrCanceled is returned when the user cancels or closes the flow.
var ErrCanceled = errors.New("sign-in was canceled")

func (p Provider) config(c Client, redirect string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.ID,
		ClientSecret: c.Secret,
		Endpoint:     oauth2.Endpoint{AuthURL: p.AuthURL, TokenURL: p.TokenURL, AuthStyle: oauth2.AuthStyleInParams},
		Scopes:       p.Scopes,
		RedirectURL:  redirect,
	}
}

// SignIn runs the browser flow. openBrowser is called with the URL to open;
// loginHint pre-fills the account on the provider's page. It returns when the
// user finishes, the context is canceled, or after a 5-minute timeout.
func SignIn(ctx context.Context, p Provider, c Client, loginHint string, openBrowser func(string) error) (*Result, error) {
	if c.ID == "" {
		return nil, fmt.Errorf("%s sign-in isn't set up yet", p.Name)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("couldn't start the sign-in listener: %w", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	listeners := []net.Listener{ln}
	if p.redirectHost() == "localhost" {
		// "localhost" may resolve to ::1 first; listen there too when we can.
		if ln6, err := net.Listen("tcp", fmt.Sprintf("[::1]:%d", port)); err == nil {
			defer ln6.Close()
			listeners = append(listeners, ln6)
		}
	}
	path := p.redirectPath()
	redirect := fmt.Sprintf("http://%s:%d%s", p.redirectHost(), port, path)
	cfg := p.config(c, redirect)

	state := randomString()
	verifier := oauth2.GenerateVerifier()
	opts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier)}
	for k, v := range p.AuthParams {
		opts = append(opts, oauth2.SetAuthURLParam(k, v))
	}
	if loginHint != "" {
		opts = append(opts, oauth2.SetAuthURLParam("login_hint", loginHint))
	}
	authURL := cfg.AuthCodeURL(state, opts...)

	type outcome struct {
		code string
		err  error
	}
	done := make(chan outcome, 1)
	srv := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != path {
				http.NotFound(w, r)
				return
			}
			q := r.URL.Query()
			var res outcome
			switch {
			case q.Get("state") != state:
				res.err = errors.New("sign-in response didn't match this request")
			case q.Get("error") == "access_denied":
				res.err = ErrCanceled
			case q.Get("error") != "":
				res.err = providerError(p.Name, q.Get("error"), q.Get("error_description"))
			case q.Get("code") == "":
				res.err = errors.New("sign-in response had no authorization code")
			default:
				res.code = q.Get("code")
			}
			writePage(w, p.Name, res.err)
			select {
			case done <- res:
			default: // only the first response counts
			}
		}),
	}
	for _, l := range listeners {
		go srv.Serve(l)
	}
	defer srv.Close()

	if err := openBrowser(authURL); err != nil {
		return nil, fmt.Errorf("couldn't open your browser: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	var res outcome
	select {
	case res = <-done:
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, errors.New("sign-in timed out. Try again")
		}
		return nil, ErrCanceled
	}
	if res.err != nil {
		return nil, res.err
	}

	tok, err := cfg.Exchange(ctx, res.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("couldn't finish signing in with %s: %w", p.Name, err)
	}
	if tok.RefreshToken == "" {
		return nil, fmt.Errorf("%s didn't grant offline access. Try again", p.Name)
	}
	var email, name string
	if p.UserInfoURL != "" {
		email, name, err = userInfo(ctx, p, tok)
	} else {
		email, name, err = idTokenIdentity(tok)
	}
	if err != nil {
		return nil, err
	}
	return &Result{Token: tok, Email: email, Name: name}, nil
}

// providerError turns an OAuth error redirect into a message people can act on.
func providerError(provider, code, desc string) error {
	// Microsoft: these AADSTS codes mean an admin must approve the app first.
	for _, c := range []string{"AADSTS65001", "AADSTS90094", "AADSTS90099"} {
		if strings.Contains(desc, c) {
			return errors.New("your organization requires an administrator to approve Pigeon before you can sign in. Ask your IT admin to allow it")
		}
	}
	if desc == "" {
		return fmt.Errorf("%s returned an error: %s", provider, code)
	}
	// Keep the first line; Microsoft appends trace and correlation IDs.
	if i := strings.IndexAny(desc, "\r\n"); i > 0 {
		desc = desc[:i]
	}
	if len(desc) > 300 {
		desc = desc[:300]
	}
	return fmt.Errorf("%s returned an error: %s", provider, desc)
}

// idTokenIdentity reads the address and name from the OpenID Connect ID token
// that came back with the access token. It arrived directly from the
// provider's token endpoint over TLS, so its signature doesn't need checking
// here (OpenID Connect Core 1.0, section 3.1.3.7).
func idTokenIdentity(tok *oauth2.Token) (email, name string, err error) {
	bad := errors.New("couldn't read your account details from the sign-in")
	raw, _ := tok.Extra("id_token").(string)
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return "", "", bad
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", bad
	}
	var claims struct {
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", "", bad
	}
	email = strings.TrimSpace(claims.Email)
	if email == "" && strings.Contains(claims.PreferredUsername, "@") {
		email = strings.TrimSpace(claims.PreferredUsername)
	}
	if email == "" {
		return "", "", errors.New("this account has no email address")
	}
	return email, strings.TrimSpace(claims.Name), nil
}

func userInfo(ctx context.Context, p Provider, tok *oauth2.Token) (email, name string, err error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.UserInfoURL, nil)
	tok.SetAuthHeader(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("couldn't read your account details: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("couldn't read your account details (%s)", resp.Status)
	}
	var info struct {
		Email         string `json:"email"`
		EmailVerified *bool  `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", "", err
	}
	if info.Email == "" || (info.EmailVerified != nil && !*info.EmailVerified) {
		return "", "", errors.New("this account has no verified email address")
	}
	return info.Email, info.Name, nil
}

// TokenSource returns a source that refreshes tok when it expires.
// onRefresh is called with each new token so it can be saved.
func TokenSource(ctx context.Context, p Provider, c Client, tok *oauth2.Token, onRefresh func(*oauth2.Token)) oauth2.TokenSource {
	base := p.config(c, "").TokenSource(ctx, tok)
	return &notifyingSource{base: base, last: tok.AccessToken, onRefresh: onRefresh}
}

type notifyingSource struct {
	base      oauth2.TokenSource
	last      string
	onRefresh func(*oauth2.Token)
}

func (s *notifyingSource) Token() (*oauth2.Token, error) {
	t, err := s.base.Token()
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) && re.ErrorCode == "invalid_grant" {
			return nil, ErrExpired
		}
		return nil, err
	}
	if t.AccessToken != s.last {
		s.last = t.AccessToken
		if s.onRefresh != nil {
			s.onRefresh(t)
		}
	}
	return t, nil
}

// ErrExpired means the grant was revoked or expired; the user must sign in again.
var ErrExpired = errors.New("sign-in expired. Sign in again from Settings → Accounts")

func randomString() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// writePage shows a minimal, self-contained result page in the browser tab.
func writePage(w http.ResponseWriter, provider string, err error) {
	title, msg := "You're signed in", "You can close this tab and go back to Pigeon."
	if errors.Is(err, ErrCanceled) {
		title, msg = "Sign-in canceled", "You can close this tab. Nothing was connected."
	} else if err != nil {
		title, msg = "Sign-in didn't finish", err.Error()
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><title>Pigeon</title>
<style>body{font:15px/1.5 "Segoe UI",system-ui,sans-serif;display:grid;place-items:center;height:100vh;margin:0;background:#f7f7f8;color:#17171c}
@media (prefers-color-scheme:dark){body{background:#131417;color:#ececf0}}
main{text-align:center;max-width:420px;padding:24px}h1{font-size:20px;margin:0 0 8px}p{margin:0;opacity:.7}</style></head>
<body><main><h1>%s</h1><p>%s</p><p style="margin-top:16px;font-size:13px">Pigeon · %s</p></main></body></html>`,
		html.EscapeString(title), html.EscapeString(msg), html.EscapeString(provider))
}
