// Package settings persists user preferences as JSON in the OS config
// directory (%AppData%\Pigeon on Windows). Secrets never go here; see
// internal/secrets.
package settings

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// Theme is the user's appearance choice.
type Theme string

const (
	ThemeSystem Theme = "system"
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
)

// Valid reports whether t is a known theme.
func (t Theme) Valid() bool {
	return t == ThemeSystem || t == ThemeLight || t == ThemeDark
}

// Settings are the user's preferences.
type Settings struct {
	Theme Theme `json:"theme"`
	// GoogleClientID is the user's own Google OAuth client (see docs/google-sign-in.md).
	// Its secret is kept in the OS keychain, not here.
	GoogleClientID string `json:"googleClientId,omitempty"`
}

func defaults() Settings {
	return Settings{Theme: ThemeSystem}
}

// Store loads and saves settings to a file.
type Store struct {
	mu   sync.Mutex
	path string
	cur  Settings
}

// Open loads settings from the default location, falling back to defaults
// if the file is missing or unreadable.
func Open() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		// Still usable in memory; saving will fail with a clear error.
		return &Store{cur: defaults()}, err
	}
	return OpenAt(filepath.Join(dir, "Pigeon", "settings.json"))
}

// OpenAt loads settings from path.
func OpenAt(path string) (*Store, error) {
	s := &Store{path: path, cur: defaults()}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	var loaded Settings
	if err := json.Unmarshal(data, &loaded); err != nil {
		return s, err // keep defaults; a corrupt file must not stop the app
	}
	if loaded.Theme.Valid() {
		s.cur.Theme = loaded.Theme
	}
	s.cur.GoogleClientID = loaded.GoogleClientID
	return s, nil
}

// Get returns the current settings.
func (s *Store) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

// Update applies fn to a copy of the settings and saves the result.
func (s *Store) Update(fn func(*Settings)) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cur
	fn(&next)
	if err := s.save(next); err != nil {
		return s.cur, err
	}
	s.cur = next
	return next, nil
}

// save writes atomically: temp file, then rename.
func (s *Store) save(v Settings) error {
	if s.path == "" {
		return errors.New("settings: no config directory available")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
