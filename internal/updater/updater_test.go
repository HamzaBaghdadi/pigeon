package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type fakeRelease struct {
	tag        string
	installer  []byte
	sums       []byte
	sig        []byte
	draft      bool
	noReleases bool
}

func newKey(t *testing.T) (string, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(pub), priv
}

// signedRelease builds a release whose checksum list is signed with priv.
func signedRelease(tag string, installer []byte, priv ed25519.PrivateKey) *fakeRelease {
	sum := sha256.Sum256(installer)
	name := "Pigeon-" + tag + "-windows-setup.exe"
	sums := []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), name))
	sig := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, sums)))
	return &fakeRelease{tag: tag, installer: installer, sums: sums, sig: sig}
}

func serve(t *testing.T, r *fakeRelease) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, _ *http.Request) {
		if r.noReleases {
			http.NotFound(w, nil)
			return
		}
		name := "Pigeon-" + r.tag + "-windows-setup.exe"
		json.NewEncoder(w).Encode(map[string]any{
			"tag_name": r.tag, "body": "Notes", "html_url": srv.URL + "/page", "draft": r.draft,
			"assets": []map[string]string{
				{"name": name, "browser_download_url": srv.URL + "/installer"},
				{"name": "Pigeon-" + r.tag + "-windows-portable.zip", "browser_download_url": srv.URL + "/zip"},
				{"name": "SHA256SUMS.txt", "browser_download_url": srv.URL + "/sums"},
				{"name": "SHA256SUMS.txt.sig", "browser_download_url": srv.URL + "/sig"},
			},
		})
	})
	mux.HandleFunc("/installer", func(w http.ResponseWriter, _ *http.Request) { w.Write(r.installer) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, _ *http.Request) { w.Write(r.sums) })
	mux.HandleFunc("/sig", func(w http.ResponseWriter, _ *http.Request) { w.Write(r.sig) })
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newUpdater(t *testing.T, srv *httptest.Server, current, pub string) *Updater {
	return &Updater{APIURL: srv.URL + "/latest", Current: current, PublicKey: pub,
		AssetSuffix: "-windows-setup.exe", Dir: t.TempDir(), allowHTTP: true}
}

func TestUpdateHappyPath(t *testing.T) {
	pub, priv := newKey(t)
	payload := []byte("MZ fake installer v0.2.0")
	srv := serve(t, signedRelease("v0.2.0", payload, priv))
	u := newUpdater(t, srv, "0.1.0", pub)

	rel, err := u.Check(context.Background())
	if err != nil || rel == nil || rel.Version != "0.2.0" {
		t.Fatalf("Check = %+v, %v", rel, err)
	}
	path, err := u.Download(context.Background(), rel)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(payload) || !strings.HasSuffix(path, "Pigeon-v0.2.0-windows-setup.exe") {
		t.Errorf("downloaded %q to %s", got, path)
	}
	// A second download reuses the verified file.
	if again, err := u.Download(context.Background(), rel); err != nil || again != path {
		t.Errorf("re-download = %s, %v", again, err)
	}
}

func TestUpToDateAndNoRelease(t *testing.T) {
	pub, priv := newKey(t)
	srv := serve(t, signedRelease("v0.1.0", []byte("x"), priv))
	if rel, err := newUpdater(t, srv, "0.1.0", pub).Check(context.Background()); err != nil || rel != nil {
		t.Errorf("same version: %+v, %v", rel, err)
	}
	if rel, err := newUpdater(t, srv, "0.2.0", pub).Check(context.Background()); err != nil || rel != nil {
		t.Errorf("older release offered: %+v, %v", rel, err)
	}
	none := serve(t, &fakeRelease{noReleases: true})
	if rel, err := newUpdater(t, none, "0.1.0", pub).Check(context.Background()); err != nil || rel != nil {
		t.Errorf("no releases: %+v, %v", rel, err)
	}
	draft := signedRelease("v9.0.0", []byte("x"), priv)
	draft.draft = true
	if rel, _ := newUpdater(t, serve(t, draft), "0.1.0", pub).Check(context.Background()); rel != nil {
		t.Error("draft release offered")
	}
}

func TestRejectsTamperedInstaller(t *testing.T) {
	pub, priv := newKey(t)
	r := signedRelease("v0.2.0", []byte("genuine"), priv)
	r.installer = []byte("malware") // swapped after signing
	u := newUpdater(t, serve(t, r), "0.1.0", pub)
	rel, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.Download(context.Background(), rel); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("tampered installer accepted: %v", err)
	}
	if entries, _ := os.ReadDir(u.Dir); len(entries) != 0 {
		t.Errorf("rejected download left files behind: %v", entries)
	}
}

func TestRejectsWrongSigner(t *testing.T) {
	pub, _ := newKey(t)
	_, attacker := newKey(t)
	// Attacker re-signs a matching checksum list with their own key.
	u := newUpdater(t, serve(t, signedRelease("v0.2.0", []byte("malware"), attacker)), "0.1.0", pub)
	rel, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.Download(context.Background(), rel); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("wrong signer accepted: %v", err)
	}
}

func TestNoPublicKeyMeansNoUpdates(t *testing.T) {
	_, priv := newKey(t)
	u := newUpdater(t, serve(t, signedRelease("v0.2.0", []byte("x"), priv)), "0.1.0", "")
	if _, err := u.Check(context.Background()); err != ErrNotConfigured {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestRejectsInsecureLinks(t *testing.T) {
	pub, priv := newKey(t)
	u := newUpdater(t, serve(t, signedRelease("v0.2.0", []byte("x"), priv)), "0.1.0", pub)
	u.allowHTTP = false
	if _, err := u.Check(context.Background()); err == nil {
		t.Fatal("accepted http:// download links")
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.2.0", "0.1.0", true}, {"0.1.0", "0.2.0", false}, {"0.1.0", "0.1.0", false},
		{"1.0.0", "0.9.9", true}, {"0.10.0", "0.9.0", true}, {"v1.2.3", "1.2.2", true},
		{"1.2.0", "1.2.0-beta.1", true}, {"1.2.0-beta.1", "1.2.0", false},
		{"garbage", "0.1.0", false}, {"0.1.0", "dev", true},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
