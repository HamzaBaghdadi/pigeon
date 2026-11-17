package services

import (
	"errors"
	"strings"

	"github.com/HamzaBaghdadi/pigeon/internal/settings"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// SettingsService exposes user preferences to the frontend.
type SettingsService struct {
	store  *settings.Store
	window *application.WebviewWindow
}

// NewSettingsService wraps a settings store.
func NewSettingsService(store *settings.Store) *SettingsService {
	return &SettingsService{store: store}
}

// AttachWindow lets the service keep the native window in sync with the theme.
func (s *SettingsService) AttachWindow(w *application.WebviewWindow) {
	s.window = w
}

// GetSettings returns the current preferences.
func (s *SettingsService) GetSettings() settings.Settings {
	return s.store.Get()
}

// SetTheme saves the appearance choice ("system", "light" or "dark") and
// updates the native window chrome to match.
func (s *SettingsService) SetTheme(theme settings.Theme) (settings.Settings, error) {
	if !theme.Valid() {
		return s.store.Get(), errors.New("unknown theme: " + string(theme))
	}
	next, err := s.store.Update(func(v *settings.Settings) { v.Theme = theme })
	if err != nil {
		return next, err
	}
	s.ApplyWindowTheme()
	return next, nil
}

// ApplyWindowTheme pushes the saved theme to the window's title bar and background.
func (s *SettingsService) ApplyWindowTheme() {
	if s.window == nil {
		return
	}
	dark := ResolveDark(s.store.Get().Theme)
	w := s.window
	application.InvokeAsync(func() {
		setNativeDark(w, dark)
		w.SetBackgroundColour(BackgroundFor(dark))
	})
}

// WindowTheme maps a saved theme to the Wails window option used at startup.
func WindowTheme(t settings.Theme) application.Theme {
	switch t {
	case settings.ThemeLight:
		return application.Light
	case settings.ThemeDark:
		return application.Dark
	default:
		return application.SystemDefault
	}
}

// ResolveDark reports whether the theme currently renders dark.
func ResolveDark(t settings.Theme) bool {
	switch t {
	case settings.ThemeLight:
		return false
	case settings.ThemeDark:
		return true
	default:
		return systemIsDark()
	}
}

// BackgroundFor matches the window background to the UI's --bg token so there
// is no flash of the wrong color while the page loads or resizes.
func BackgroundFor(dark bool) application.RGBA {
	if dark {
		return application.NewRGB(0x13, 0x14, 0x17)
	}
	return application.NewRGB(0xf7, 0xf7, 0xf8)
}

// SetDisplayName sets the name recipients see on mail you send.
func (s *SettingsService) SetDisplayName(name string) (settings.Settings, error) {
	name = strings.Join(strings.Fields(name), " ")
	if len(name) > 100 {
		return s.store.Get(), errors.New("that name is too long")
	}
	return s.store.Update(func(v *settings.Settings) { v.DisplayName = name })
}

// SetNotifications turns new-mail notifications on or off.
func (s *SettingsService) SetNotifications(on bool) (settings.Settings, error) {
	return s.store.Update(func(v *settings.Settings) { v.DisableNotifications = !on })
}

// SetCloseToTray chooses whether closing the window keeps Pigeon running in the tray.
func (s *SettingsService) SetCloseToTray(on bool) (settings.Settings, error) {
	return s.store.Update(func(v *settings.Settings) { v.DisableCloseToTray = !on })
}
