// Package updater keeps installed copies of Pigeon up to date from GitHub
// Releases.
//
// Security model: a release is only installed if SHA256SUMS.txt carries a
// valid Ed25519 signature from the key compiled into the app (PublicKey) and
// the installer's SHA-256 matches the signed list. A swapped or corrupted
// download can't pass that check; the private key lives only in the release
// workflow's secrets.
package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DefaultAPIURL is the latest published (non-draft, non-prerelease) release.
const DefaultAPIURL = "https://api.github.com/repos/HamzaBaghdadi/pigeon/releases/latest"

const (
	sumsName     = "SHA256SUMS.txt"
	sigName      = "SHA256SUMS.txt.sig"
	maxInstaller = 300 << 20 // refuse absurdly large downloads
	maxSmall     = 64 << 10  // sums and signature files
)

// Release is an available update.
type Release struct {
	Version     string    `json:"version"`
	Notes       string    `json:"notes"`
	URL         string    `json:"url"` // release page
	PublishedAt time.Time `json:"publishedAt"`

	installerName string
	installerURL  string
	sumsURL       string
	sigURL        string
}

// Updater checks for and downloads updates.
type Updater struct {
	APIURL      string
	Current     string // running version, e.g. "0.1.0"
	PublicKey   string // base64 Ed25519 public key
	AssetSuffix string // e.g. "-windows-setup.exe"
	Dir         string // where downloads go
	Client      *http.Client
	allowHTTP   bool // tests only
}

// ErrNotConfigured means this build can't verify updates, so it won't install any.
var ErrNotConfigured = errors.New("updates aren't set up for this build")

func (u *Updater) client() *http.Client {
	if u.Client != nil {
		return u.Client
	}
	return &http.Client{Timeout: 10 * time.Minute}
}

// Check returns the latest release if it is newer than the running version,
// or nil if Pigeon is up to date.
func (u *Updater) Check(ctx context.Context) (*Release, error) {
	if u.PublicKey == "" {
		return nil, ErrNotConfigured
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.APIURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Pigeon-Updater") // no user or device identifiers
	resp, err := u.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("couldn't reach GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // no published release yet
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub returned %s", resp.Status)
	}
	var gh struct {
		TagName     string    `json:"tag_name"`
		Body        string    `json:"body"`
		HTMLURL     string    `json:"html_url"`
		Draft       bool      `json:"draft"`
		Prerelease  bool      `json:"prerelease"`
		PublishedAt time.Time `json:"published_at"`
		Assets      []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&gh); err != nil {
		return nil, fmt.Errorf("couldn't read release info: %w", err)
	}
	if gh.Draft || gh.Prerelease {
		return nil, nil
	}
	latest := strings.TrimPrefix(gh.TagName, "v")
	if !Newer(latest, u.Current) {
		return nil, nil
	}
	r := &Release{Version: latest, Notes: gh.Body, URL: gh.HTMLURL, PublishedAt: gh.PublishedAt}
	for _, a := range gh.Assets {
		switch {
		case a.Name == sumsName:
			r.sumsURL = a.URL
		case a.Name == sigName:
			r.sigURL = a.URL
		case strings.HasSuffix(a.Name, u.AssetSuffix) && validName(a.Name):
			r.installerName, r.installerURL = a.Name, a.URL
		}
	}
	if r.installerURL == "" || r.sumsURL == "" || r.sigURL == "" {
		return nil, fmt.Errorf("release %s is missing its installer or signature", gh.TagName)
	}
	for _, link := range []string{r.installerURL, r.sumsURL, r.sigURL} {
		if !u.allowHTTP && !strings.HasPrefix(link, "https://") {
			return nil, fmt.Errorf("refusing insecure download link")
		}
	}
	return r, nil
}

// Download fetches and verifies the release installer and returns its path.
// Nothing is returned unless the signature and checksum both verify.
func (u *Updater) Download(ctx context.Context, r *Release) (string, error) {
	sums, err := u.fetchSmall(ctx, r.sumsURL)
	if err != nil {
		return "", err
	}
	sig, err := u.fetchSmall(ctx, r.sigURL)
	if err != nil {
		return "", err
	}
	if err := Verify(u.PublicKey, sums, sig); err != nil {
		return "", err
	}
	want, ok := checksumFor(sums, r.installerName)
	if !ok {
		return "", fmt.Errorf("the signed checksum list doesn't include %s", r.installerName)
	}

	if err := os.MkdirAll(u.Dir, 0o700); err != nil {
		return "", err
	}
	final := filepath.Join(u.Dir, r.installerName)
	if have, err := fileSHA256(final); err == nil && have == want {
		return final, nil // already downloaded and intact
	}
	tmp, err := os.CreateTemp(u.Dir, "download-*.part")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.installerURL, nil)
	if err != nil {
		tmp.Close()
		return "", err
	}
	req.Header.Set("User-Agent", "Pigeon-Updater")
	resp, err := u.client().Do(req)
	if err != nil {
		tmp.Close()
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		return "", fmt.Errorf("download failed: %s", resp.Status)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, maxInstaller+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	if n > maxInstaller {
		return "", errors.New("download is unexpectedly large; not installing")
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return "", errors.New("downloaded file doesn't match the signed checksum; not installing")
	}
	_ = os.Remove(final)
	if err := os.Rename(tmp.Name(), final); err != nil {
		return "", err
	}
	return final, nil
}

// Verify checks a base64 Ed25519 signature over data.
func Verify(publicKey string, data, sigB64 []byte) error {
	pub, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return ErrNotConfigured
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigB64)))
	if err != nil || !ed25519.Verify(ed25519.PublicKey(pub), data, sig) {
		return errors.New("the update's signature is invalid; not installing")
	}
	return nil
}

// Cleanup removes downloaded installers other than keep.
func (u *Updater) Cleanup(keep string) {
	entries, _ := os.ReadDir(u.Dir)
	for _, e := range entries {
		p := filepath.Join(u.Dir, e.Name())
		if p != keep {
			_ = os.Remove(p)
		}
	}
}

func (u *Updater) fetchSmall(ctx context.Context, link string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Pigeon-Updater")
	resp, err := u.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxSmall))
}

// checksumFor finds "hash  name" in a sha256sum-style list.
func checksumFor(sums []byte, name string) (string, bool) {
	for _, line := range strings.Split(string(sums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name && len(f[0]) == 64 {
			return strings.ToLower(f[0]), true
		}
	}
	return "", false
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// validName rejects asset names that could escape the download folder.
func validName(n string) bool {
	return n != "" && !strings.ContainsAny(n, `/\:`) && !strings.Contains(n, "..")
}

// Newer reports whether version a is newer than b (semantic versioning;
// a pre-release like 1.2.0-beta.1 is older than 1.2.0).
func Newer(a, b string) bool {
	pa, preA, okA := parse(a)
	pb, preB, okB := parse(b)
	if !okA {
		return false
	}
	if !okB {
		return true // unknown current version: offer the release
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	switch {
	case preA == preB:
		return false
	case preA == "":
		return true
	case preB == "":
		return false
	default:
		return preA > preB
	}
}

func parse(v string) ([3]int, string, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	core, pre, _ := strings.Cut(v, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return out, "", false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, "", false
		}
		out[i] = n
	}
	return out, pre, true
}
