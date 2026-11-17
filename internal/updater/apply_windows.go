//go:build windows

package updater

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
)

// AssetSuffix is the release asset this platform installs.
const AssetSuffix = "-windows-setup.exe"

// Installed reports whether Pigeon runs from the installer's location (as
// opposed to the portable zip), i.e. whether it can update itself in place.
func Installed(exe string) bool {
	return installedAt(exe, os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramFiles"))
}

func installedAt(exe, localAppData, programFiles string) bool {
	dir := strings.ToLower(filepath.Clean(filepath.Dir(exe)))
	for _, base := range []string{
		filepath.Join(localAppData, "Programs", "Pigeon"), // per-user install
		filepath.Join(programFiles, "Pigeon", "Pigeon"),   // per-machine install
	} {
		if base != "" && dir == strings.ToLower(filepath.Clean(base)) {
			return true
		}
	}
	return false
}

// Apply starts a hidden helper that waits for this process to exit, runs the
// verified installer silently, and reopens Pigeon. The caller should quit
// right after Apply returns.
func Apply(installer, exe string) error {
	if !Installed(exe) {
		return fmt.Errorf("this copy of Pigeon isn't installed, so it can't update itself")
	}
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	logPath := filepath.Join(filepath.Dir(installer), "update.log")
	script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$log = %s
function Log($m) { Add-Content -LiteralPath $log -Value ("{0:u} {1}" -f (Get-Date), $m) }
try {
  Log "waiting for Pigeon (pid %d) to exit"
  Wait-Process -Id %d -Timeout 60 -ErrorAction SilentlyContinue
  Log "running installer"
  $p = Start-Process -FilePath %s -ArgumentList '/S' -Wait -PassThru
  Log ("installer exited with code {0}" -f $p.ExitCode)
} catch { Log ("update failed: {0}" -f $_) }
# Reopen Pigeon whether or not the install succeeded.
try { Start-Process -FilePath %s; Log "relaunched Pigeon" } catch { Log ("relaunch failed: {0}" -f $_) }
`, q(logPath), os.Getpid(), os.Getpid(), q(installer), q(exe))

	// -EncodedCommand takes base64 UTF-16LE, which avoids quoting pitfalls.
	u := utf16.Encode([]rune(script))
	b := make([]byte, len(u)*2)
	for i, c := range u {
		b[2*i], b[2*i+1] = byte(c), byte(c>>8)
	}
	encoded := base64.StdEncoding.EncodeToString(b)
	start := func(flags uint32) error {
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
			"-WindowStyle", "Hidden", "-EncodedCommand", encoded)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags}
		return cmd.Start()
	}
	// The helper must outlive Pigeon. If Pigeon runs inside a job object
	// (some launchers do this), breaking away keeps the helper from being
	// killed when Pigeon exits; fall back if the job doesn't allow it.
	// CREATE_NO_WINDOW, not DETACHED_PROCESS: PowerShell exits silently
	// without a console, but runs fine with a hidden one.
	const createNoWindow, newProcessGroup, breakawayFromJob = 0x08000000, 0x00000200, 0x01000000
	if err := start(createNoWindow | newProcessGroup | breakawayFromJob); err == nil {
		return nil
	}
	return start(createNoWindow | newProcessGroup)
}
