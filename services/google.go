package services

import (
	"errors"
	"fmt"
	"strings"

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

func newGoogleAuth(prefs *settings.Store) *oauthAuth {
	return newOAuthAuth(oauth.Google, func() oauth.Client {
		if id := prefs.Get().GoogleClientID; id != "" {
			secret, _ := secrets.Get(googleSecretKey)
			return oauth.Client{ID: id, Secret: secret}
		}
		return oauth.Client{ID: GoogleClientID, Secret: GoogleClientSecret}
	})
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
	s.core.google.reset() // new client: rebuild token sources
	return err
}

// SignInWithGoogle opens Google's sign-in page in the browser and, once the
// user approves, adds the Gmail account. It returns when the flow finishes.
func (s *AccountService) SignInWithGoogle(loginHint, name string) (pmail.Account, error) {
	return s.addOAuthAccount(oauthAccountSpec{
		auth:        s.core.google,
		authKind:    store.AuthGoogle,
		provider:    pmail.ProviderGmail,
		servers:     googleServers(),
		defaultName: "Gmail",
		imapRefused: "Google signed you in, but Gmail refused IMAP access. Check that IMAP is enabled in Gmail settings",
	}, loginHint, name)
}

// CancelSignIn aborts any pending browser sign-in.
func (s *AccountService) CancelSignIn() {
	s.core.google.cancelSignIn()
	s.core.microsoft.cancelSignIn()
}

// Reconnect signs in again for an existing Google or Microsoft account whose
// access expired or was revoked.
func (s *AccountService) Reconnect(accountID string) error {
	a, err := s.core.store.GetAccount(accountID)
	if err != nil {
		return err
	}
	auth := s.core.oauthFor(a.Auth)
	if auth == nil {
		return errors.New("this account signs in with a password")
	}
	res, err := auth.signIn(a.Email, openBrowser)
	if err != nil {
		return err
	}
	if !strings.EqualFold(res.Email, a.Email) {
		return fmt.Errorf("you signed in as %s, but this account is %s", res.Email, a.Email)
	}
	if err := saveToken(a.ID, res.Token); err != nil {
		return err
	}
	auth.forget(a.ID)
	s.core.engine.Disconnect(a.ID)
	s.core.SyncSoon()
	return nil
}

type oauthAccountSpec struct {
	auth        *oauthAuth
	authKind    string
	provider    pmail.Provider
	servers     store.Account
	defaultName string // name for the first account of this kind
	imapRefused string // shown when sign-in worked but IMAP said no
}

// addOAuthAccount runs the browser sign-in, checks IMAP accepts the token,
// then saves the account and its token.
func (s *AccountService) addOAuthAccount(spec oauthAccountSpec, loginHint, name string) (pmail.Account, error) {
	res, err := spec.auth.signIn(strings.TrimSpace(loginHint), openBrowser)
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

	acct := spec.servers
	if err := imapsync.Verify(imapsync.Config{
		Host: acct.IMAPHost, Port: acct.IMAPPort, Security: imapsync.Security(acct.IMAPSecurity),
		Username: res.Email, AccessToken: res.Token.AccessToken,
	}); err != nil {
		if errors.Is(err, imapsync.ErrAuth) {
			return pmail.Account{}, errors.New(spec.imapRefused)
		}
		return pmail.Account{}, err
	}

	if strings.TrimSpace(name) == "" {
		name = spec.defaultName
		if realCount > 0 {
			name = res.Email
		}
	}
	acct.Account = pmail.Account{
		ID: newID(), Name: strings.TrimSpace(name), Email: res.Email, Provider: spec.provider,
		Color: accountColors[realCount%len(accountColors)],
	}
	acct.Username = res.Email
	acct.Auth = spec.authKind

	if s.core.prefs.Get().DisplayName == "" && strings.TrimSpace(res.Name) != "" {
		_, _ = s.core.prefs.Update(func(v *settings.Settings) { v.DisplayName = strings.TrimSpace(res.Name) })
	}
	if err := saveToken(acct.ID, res.Token); err != nil {
		return pmail.Account{}, fmt.Errorf("couldn't save the sign-in to Windows Credential Manager: %w", err)
	}
	if err := s.core.store.SaveAccount(acct); err != nil {
		_ = secrets.Delete(oauthKey(acct.ID))
		return pmail.Account{}, err
	}
	if realCount == 0 {
		_ = s.core.store.DeleteDemoAccounts()
	}
	emit(EventMailChanged, "")
	s.core.SyncSoon()
	return acct.Account, nil
}

func googleServers() store.Account {
	return store.Account{
		IMAPHost: "imap.gmail.com", IMAPPort: 993, IMAPSecurity: "tls",
		SMTPHost: "smtp.gmail.com", SMTPPort: 465, SMTPSecurity: "tls",
	}
}
