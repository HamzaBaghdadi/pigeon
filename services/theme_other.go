//go:build !windows

package services

import "github.com/wailsapp/wails/v3/pkg/application"

// TODO(macos, linux): detect the system theme and switch native chrome.
func systemIsDark() bool { return false }

func setNativeDark(*application.WebviewWindow, bool) {}
