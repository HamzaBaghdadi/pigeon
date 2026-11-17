package services

import (
	"context"
	"encoding/json"
	"sync"

	"golang.org/x/oauth2"

	"github.com/HamzaBaghdadi/pigeon/internal/oauth"
	"github.com/HamzaBaghdadi/pigeon/internal/secrets"
)

func oauthKey(accountID string) string { return "account:" + accountID + ":oauth" }

// oauthAuth manages one provider's OAuth client and per-account token sources.
type oauthAuth struct {
	provider oauth.Provider
	client   func() oauth.Client

	mu      sync.Mutex
	sources map[string]oauth2.TokenSource
	cancel  context.CancelFunc // pending browser sign-in, if any
}

func newOAuthAuth(p oauth.Provider, client func() oauth.Client) *oauthAuth {
	return &oauthAuth{provider: p, client: client, sources: map[string]oauth2.TokenSource{}}
}

// configured reports whether this build or the user has a client for the provider.
func (g *oauthAuth) configured() bool { return g.client().ID != "" }

// accessToken returns a valid access token for an account, refreshing and
// saving it when it has expired.
func (g *oauthAuth) accessToken(accountID string) (string, error) {
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
		ts = oauth.TokenSource(context.Background(), g.provider, g.client(), &tok, func(t *oauth2.Token) {
			// Microsoft rotates refresh tokens, so always keep the latest.
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

func (g *oauthAuth) forget(accountID string) {
	g.mu.Lock()
	delete(g.sources, accountID)
	g.mu.Unlock()
}

func (g *oauthAuth) reset() {
	g.mu.Lock()
	g.sources = map[string]oauth2.TokenSource{}
	g.mu.Unlock()
}

// signIn runs the browser flow; only one can be pending at a time per provider.
func (g *oauthAuth) signIn(loginHint string, openBrowser func(string) error) (*oauth.Result, error) {
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
	return oauth.SignIn(ctx, g.provider, g.client(), loginHint, openBrowser)
}

func (g *oauthAuth) cancelSignIn() {
	g.mu.Lock()
	if g.cancel != nil {
		g.cancel()
	}
	g.mu.Unlock()
}

// maxTokenEntry stays under Windows Credential Manager's 2,560-byte limit.
const maxTokenEntry = 2400

func saveToken(accountID string, t *oauth2.Token) error {
	// Keep only what's needed; the keychain has a small size limit per entry.
	b, err := json.Marshal(&oauth2.Token{AccessToken: t.AccessToken, TokenType: t.TokenType, RefreshToken: t.RefreshToken, Expiry: t.Expiry})
	if err != nil {
		return err
	}
	if len(b) > maxTokenEntry {
		// Microsoft access tokens can be large. Keep just the refresh token;
		// a fresh access token is fetched on the next connection.
		b, err = json.Marshal(&oauth2.Token{RefreshToken: t.RefreshToken})
		if err != nil {
			return err
		}
	}
	return secrets.Set(oauthKey(accountID), string(b))
}
