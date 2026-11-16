package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// fakeProvider is a minimal OAuth server: it checks PKCE and returns tokens.
func fakeProvider(t *testing.T) (Provider, *string) {
	t.Helper()
	lastVerifier := new(string)
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		*lastVerifier = r.Form.Get("code_verifier")
		if r.Form.Get("code") != "good-code" || r.Form.Get("client_id") != "cid" || *lastVerifier == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-1", "refresh_token": "rt-1", "token_type": "Bearer", "expires_in": 3600,
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"email": "me@gmail.com", "email_verified": true, "name": "Me"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := Provider{
		Name: "Fake", AuthURL: srv.URL + "/auth", TokenURL: srv.URL + "/token", UserInfoURL: srv.URL + "/userinfo",
		Scopes: []string{"mail"}, AuthParams: map[string]string{"access_type": "offline"},
	}
	return p, lastVerifier
}

// browser simulates the user approving (or not) on the provider page.
func browser(t *testing.T, code, errParam string, tamperState bool, seen *url.Values) func(string) error {
	return func(authURL string) error {
		u, _ := url.Parse(authURL)
		q := u.Query()
		*seen = q
		cb, _ := url.Parse(q.Get("redirect_uri"))
		v := url.Values{}
		state := q.Get("state")
		if tamperState {
			state = "attacker"
		}
		v.Set("state", state)
		if code != "" {
			v.Set("code", code)
		}
		if errParam != "" {
			v.Set("error", errParam)
		}
		cb.RawQuery = v.Encode()
		go func() {
			time.Sleep(20 * time.Millisecond)
			resp, err := http.Get(cb.String())
			if err == nil {
				resp.Body.Close()
			}
		}()
		return nil
	}
}

func TestSignInSuccess(t *testing.T) {
	p, _ := fakeProvider(t)
	var seen url.Values
	res, err := SignIn(context.Background(), p, Client{ID: "cid", Secret: "s"}, "me@gmail.com", browser(t, "good-code", "", false, &seen))
	if err != nil {
		t.Fatalf("SignIn: %v", err)
	}
	if res.Email != "me@gmail.com" || res.Token.RefreshToken != "rt-1" {
		t.Errorf("unexpected result: %+v", res)
	}
	// The request must carry PKCE, offline access, the hint and a loopback redirect.
	if seen.Get("code_challenge_method") != "S256" || seen.Get("code_challenge") == "" {
		t.Errorf("missing PKCE: %v", seen)
	}
	if seen.Get("access_type") != "offline" || seen.Get("login_hint") != "me@gmail.com" {
		t.Errorf("missing params: %v", seen)
	}
	if r, _ := url.Parse(seen.Get("redirect_uri")); r.Hostname() != "127.0.0.1" {
		t.Errorf("redirect must be loopback, got %s", seen.Get("redirect_uri"))
	}
}

func TestSignInRejectsWrongState(t *testing.T) {
	p, _ := fakeProvider(t)
	var seen url.Values
	_, err := SignIn(context.Background(), p, Client{ID: "cid"}, "", browser(t, "good-code", "", true, &seen))
	if err == nil {
		t.Fatal("accepted a response with the wrong state")
	}
}

func TestSignInDenied(t *testing.T) {
	p, _ := fakeProvider(t)
	var seen url.Values
	_, err := SignIn(context.Background(), p, Client{ID: "cid"}, "", browser(t, "", "access_denied", false, &seen))
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("err = %v, want ErrCanceled", err)
	}
}

func TestSignInCancel(t *testing.T) {
	p, _ := fakeProvider(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	_, err := SignIn(ctx, p, Client{ID: "cid"}, "", func(string) error { return nil })
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("err = %v, want ErrCanceled", err)
	}
}

func TestSignInNeedsClientID(t *testing.T) {
	if _, err := SignIn(context.Background(), Google, Client{}, "", func(string) error { return nil }); err == nil {
		t.Fatal("expected an error without a client ID")
	}
}

func TestTokenSourceRefreshAndExpiry(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.Form.Get("refresh_token") == "revoked" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "fresh", "token_type": "Bearer", "expires_in": 3600})
	}))
	defer srv.Close()
	p := Provider{TokenURL: srv.URL}

	var saved *oauth2.Token
	expired := &oauth2.Token{AccessToken: "old", RefreshToken: "rt", Expiry: time.Now().Add(-time.Hour)}
	ts := TokenSource(context.Background(), p, Client{ID: "cid"}, expired, func(t *oauth2.Token) { saved = t })
	tok, err := ts.Token()
	if err != nil || tok.AccessToken != "fresh" || saved == nil || saved.RefreshToken != "rt" {
		t.Fatalf("refresh: tok=%+v saved=%+v err=%v", tok, saved, err)
	}

	revoked := &oauth2.Token{AccessToken: "old", RefreshToken: "revoked", Expiry: time.Now().Add(-time.Hour)}
	_, err = TokenSource(context.Background(), p, Client{ID: "cid"}, revoked, nil).Token()
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("err = %v, want ErrExpired", err)
	}
}
