//go:build windows

package updater

import (
	"os"
	"path/filepath"
	"testing"
)

// TestApplyEndToEnd exercises the real install-and-restart helper. It needs
// Pigeon installed per-user and modifies that install, so it only runs when
// asked:
//
//	PIGEON_E2E_INSTALLER=path\to\pigeon-amd64-installer.exe go test ./internal/updater -run ApplyEndToEnd
func TestApplyEndToEnd(t *testing.T) {
	installer := os.Getenv("PIGEON_E2E_INSTALLER")
	if installer == "" {
		t.Skip("set PIGEON_E2E_INSTALLER to run")
	}
	exe := filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Pigeon", "pigeon.exe")
	if !Installed(exe) {
		t.Fatalf("%s isn't recognised as an installed copy", exe)
	}
	if _, err := os.Stat(exe); err != nil {
		t.Fatalf("install Pigeon per-user first: %v", err)
	}
	// The helper waits for this test process to exit, then installs and relaunches.
	if err := Apply(installer, exe); err != nil {
		t.Fatalf("Apply: %v", err)
	}
}
