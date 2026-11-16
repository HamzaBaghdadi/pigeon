//go:build windows

package services

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"
)

func systemIsDark() bool {
	return w32.IsCurrentlyDarkMode()
}

// setNativeDark switches the Windows title bar between light and dark.
// Must run on the main thread.
func setNativeDark(w *application.WebviewWindow, dark bool) {
	hwnd := uintptr(w.NativeWindow())
	if hwnd == 0 || !w32.SupportsThemes() || w32.IsCurrentlyHighContrastMode() {
		return
	}
	if dark && w32.AllowDarkModeForWindow != nil {
		w32.AllowDarkModeForWindow(w32.HWND(hwnd), true)
	}
	w32.SetTheme(hwnd, dark)
}
