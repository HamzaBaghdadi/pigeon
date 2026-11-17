//go:build !windows

package services

// markDownloaded is a no-op outside Windows for now (macOS uses a quarantine
// extended attribute; to be added with the macOS build).
func markDownloaded(string) {}
