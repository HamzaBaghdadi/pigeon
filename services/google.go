package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/oauth2"

	"github.com/HamzaBaghdadi/pigeon/internal/imapsync"
	pmail "github.com/HamzaBaghdadi/pigeon/internal/mail"
	"github.com/HamzaBaghdadi/pigeon/internal/oauth"
	"github.com/HamzaBaghdadi/pigeon/internal/secrets"
	"github.com/HamzaBaghdadi/pigeon/internal/settings"
	"github.com/HamzaBaghdadi/pigeon/internal/store"
)

// Built-in OAuth client for official releases. Empty in source so no
// credentials live in the repository; the release workflow fills them in by
// generating the gitignored services/release_credentials.go from GitHub
// secrets (see scripts/embed-credentials and docs/releasing.md).
// A client set in Settings (stored per user) takes precedence.
var (
	GoogleClientID     string
	GoogleClientSecret string
)

const googleSecretKey = "oauth:google:client_secret"

func oauthKey(accountID string) string { return "account:" + accountID + ":oauth" }

// googleAuth manages Google OAuth clients and per-account token sources.
type googleAuth struct {
	prefs *settings.Store

	mu      sync.Mutex
	sources map[string]oauth2.TokenSource
	cancel  context.CancelFunc // pending browser sign-in, if any
}

func newGoogleAuth(prefs *settings.Store) *googleAuth {
	return &googleAuth{prefs: prefs, sources: map[string]oauth2.TokenSource{}}
}

func (g *googleAuth) client() oauth.Client {
	if id := g.prefs.Get().GoogleClientID; id != "" {
		secret, _ := secrets.Get(googleSecretKey)
		return oauth.Client{ID: id, Secret: secret}
	}
	return oauth.Client{ID: GoogleClientID, Secret: GoogleClientSecret}
}

// accessToken returns a valid access token for an account, refreshing and
// saving it when it has expired.
func (g *googleAuth) accessToken(accountID string) (string, error) {
	g.mu.Lock()
	ts := g.sources[accountID]
	if ts == nil {
		raw, err := secrets.Get(oauthKey(accountID))
		if err != nil {
			g.mu.Unlock()
			return "", oauth.ErrExpired
		}
		var tok oauth2.Token
		if err := json.Unmarshal([]byte(raw), &tok); err != nil {
			g.mu.Unlock()
			return "", oauth.ErrExpired
		}
		ts = oauth.TokenSource(context.Background(), oauth.Google, g.client(), &tok, func(t *oauth2.Token) {
			_ = saveToken(accountID, t)
		})
		g.sources[accountID] = ts
	}
	g.mu.Unlock()
	t, err := ts.Token()
	if err != nil {
		return "", err
	}
	return t.AccessToken, nil
}

func (g *googleAuth) forget(accountID string) {
	g.mu.Lock()
	delete(g.sources, accountID)
	g.mu.Unlock()
}

// signIn runs the browser flow; only one can be pending at a time.
func (g *googleAuth) signIn(loginHint string, openBrowser func(string) error) (*oauth.Result, error) {
	ctx, cancel := context.WithCancel(context.Background())
	g.mu.Lock()
	if g.cancel != nil {
		g.cancel() // a newer attempt replaces an abandoned one
	}
	g.cancel = cancel
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		g.cancel = nil
		g.mu.Unlock()
		cancel()
	}()
	return oauth.SignIn(ctx, oauth.Google, g.client(), loginHint, openBrowser)
}

func (g *googleAuth) cancelSignIn() {
	g.mu.Lock()
	if g.cancel != nil {
		g.cancel()
	}
	g.mu.Unlock()
}

func saveToken(accountID string, t *oauth2.Token) error {
	// Keep only what's needed; the keychain has a small size limit per entry.
	b, err := json.Marshal(&oauth2.Token{AccessToken: t.AccessToken, TokenType: t.TokenType, RefreshToken: t.RefreshToken, Expiry: t.Expiry})
	if err != nil {
		return err
	}
	return secrets.Set(oauthKey(accountID), string(b))
}

// ---- AccountService API ----

// GoogleSetup describes whether Sign in with Google is ready to use.
type GoogleSetup struct {
	Configured bool   `json:"configured"`
	ClientID   string `json:"clientId"`
	// BuiltIn is true when the app ships with its own client (official builds).
	BuiltIn bool `json:"builtIn"`
}

// GetGoogleSetup reports how Sign in with Google is configured.
func (s *AccountService) GetGoogleSetup() GoogleSetup {
	if id := s.core.prefs.Get().GoogleClientID; id != "" {
		return GoogleSetup{Configured: true, ClientID: id}
	}
	return GoogleSetup{Configured: GoogleClientID != "", ClientID: GoogleClientID, BuiltIn: GoogleClientID != ""}
}

// SaveGoogleSetup stores the user's own Google OAuth client. The ID goes to
// settings, the secret to the OS keychain. Empty values clear the setup.
func (s *AccountService) SaveGoogleSetup(clientID, clientSecret string) error {
	clientID, clientSecret = strings.TrimSpace(clientID), strings.TrimSpace(clientSecret)
	if clientID == "" {
		_ = secrets.Delete(googleSecretKey)
		_, err := s.core.prefs.Update(func(v *settings.Settings) { v.GoogleClientID = "" })
		return err
	}
	if !strings.HasSuffix(clientID, ".apps.googleusercontent.com") {
		return errors.New("that doesn't look like a Google client ID. It ends with .apps.googleusercontent.com")
	}
	if clientSecret == "" {
		return errors.New("paste the client secret too")
	}
	if err := secrets.Set(googleSecretKey, clientSecret); err != nil {
		return fmt.Errorf("couldn't save the client secret: %w", err)
	}
	_, err := s.core.prefs.Update(func(v *settings.Settings) { v.GoogleClientID = clientID })
	s.core.google.mu.Lock()
	s.core.google.sources = map[string]oauth2.TokenSource{} // new client: rebuild token sources
	s.core.google.mu.Unlock()
	return err
}

// SignInWithGoogle opens Google's sign-in page in the browser and, once the
// user approves, adds the Gmail account. It returns when the flow finishes.
func (s *AccountService) SignInWithGoogle(loginHint, name string) (pmail.Account, error) {
	res, err := s.core.google.signIn(strings.TrimSpace(loginHint), openBrowser)
	if err != nil {
		return pmail.Account{}, err
	}

	existing, err := s.core.store.ListAccounts()
	if err != nil {
		return pmail.Account{}, err
	}
	realCount := 0
	for _, a := range existing {
		if a.Demo {
			continue
		}
		realCount++
		if strings.EqualFold(a.Email, res.Email) {
			return pmail.Account{}, fmt.Errorf("%s is already added", res.Email)
		}
	}

	gmail := googleServers()
	if err := imapsync.Verify(imapsync.Config{
		Host: gmail.IMAPHost, Port: gmail.IMAPPort, Security: imapsync.SecurityTLS,
		Username: res.Email, AccessToken: res.Token.AccessToken,
	}); err != nil {
		if errors.Is(err, imapsync.ErrAuth) {
			return pmail.Account{}, errors.New("Google signed you in, but Gmail refused IMAP access. Check that IMAP is enabled in Gmail settings")
		}
		return pmail.Account{}, err
	}

	if strings.TrimSpace(name) == "" {
		name = "Gmail"
		if realCount > 0 {
			name = res.Email
		}
	}
	gmail.Account = pmail.Account{
		ID: newID(), Name: strings.TrimSpace(name), Email: res.Email, Provider: pmail.ProviderGmail,
		Color: accountColors[realCount%len(accountColors)],
	}
	gmail.Username = res.Email
	gmail.Auth = store.AuthGoogle

	if s.core.prefs.Get().DisplayName == "" && strings.TrimSpace(res.Name) != "" {
		_, _ = s.core.prefs.Update(func(v *settings.Settings) { v.DisplayName = strings.TrimSpace(res.Name) })
	}
	if err := saveToken(gmail.ID, res.Token); err != nil {
		return pmail.Account{}, fmt.Errorf("couldn't save the sign-in to Windows Credential Manager: %w", err)
	}
	if err := s.core.store.SaveAccount(gmail); err != nil {
		_ = secrets.Delete(oauthKey(gmail.ID))
		return pmail.Account{}, err
	}
	if realCount == 0 {
		_ = s.core.store.DeleteDemoAccounts()
	}
	emit(EventMailChanged, "")
	s.core.SyncSoon()
	return gmail.Account, nil
}

// ReconnectGoogle signs in again for an existing Google account whose access
// expired or was revoked.
func (s *AccountService) ReconnectGoogle(accountID string) error {
	a, err := s.core.store.GetAccount(accountID)
	if err != nil {
		return err
	}
	if a.Auth != store.AuthGoogle {
		return errors.New("this account doesn't use Sign in with Google")
	}
	res, err := s.core.google.signIn(a.Email, openBrowser)
	if err != nil {
		return err
	}
	if !strings.EqualFold(res.Email, a.Email) {
		return fmt.Errorf("you signed in as %s, but this account is %s", res.Email, a.Email)
	}
	if err := saveToken(a.ID, res.Token); err != nil {
		return err
	}
	s.core.google.forget(a.ID)
	s.core.engine.Disconnect(a.ID)
	s.core.SyncSoon()
	return nil
}

// CancelSignIn aborts a pending browser sign-in.
func (s *AccountService) CancelSignIn() { s.core.google.cancelSignIn() }

func googleServers() store.Account {
	return store.Account{
		IMAPHost: "imap.gmail.com", IMAPPort: 993, IMAPSecurity: "tls",
		SMTPHost: "smtp.gmail.com", SMTPPort: 465, SMTPSecurity: "tls",
	}
}
