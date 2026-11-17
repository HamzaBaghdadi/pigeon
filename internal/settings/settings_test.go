package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsWhenMissing(t *testing.T) {
	s, err := OpenAt(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get().Theme; got != ThemeSystem {
		t.Errorf("Theme = %q, want %q", got, ThemeSystem)
	}
}

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "settings.json")
	s, _ := OpenAt(path)
	if _, err := s.Update(func(v *Settings) { v.Theme = ThemeDark }); err != nil {
		t.Fatal(err)
	}
	again, err := OpenAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Get().Theme; got != ThemeDark {
		t.Errorf("Theme = %q, want %q", got, ThemeDark)
	}
}

func TestCorruptFileFallsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := OpenAt(path)
	if got := s.Get().Theme; got != ThemeSystem {
		t.Errorf("Theme = %q, want %q", got, ThemeSystem)
	}
}

func TestStyle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s, _ := OpenAt(path)
	if _, err := s.Update(func(v *Settings) { v.Style = StyleGilded }); err != nil {
		t.Fatal(err)
	}
	again, _ := OpenAt(path)
	if again.Get().Style != StyleGilded {
		t.Errorf("style = %q, want gilded", again.Get().Style)
	}

	if err := os.WriteFile(path, []byte(`{"theme":"dark","style":"neon"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	bad, _ := OpenAt(path)
	if bad.Get().Style != "" || bad.Get().Theme != ThemeDark {
		t.Errorf("unknown style should fall back to default: %+v", bad.Get())
	}
}
