//go:build !windows

package updater

import "errors"

// AssetSuffix is empty until macOS and Linux builds exist.
const AssetSuffix = "-unsupported"

// Installed is false: self-update isn't implemented on this platform yet.
func Installed(string) bool { return false }

// Apply isn't implemented on this platform yet.
func Apply(string, string) error {
	return errors.New("automatic updates aren't available on this platform yet")
}
