//go:build windows

package updater

import "testing"

func TestInstalledAt(t *testing.T) {
	local, pf := `C:\Users\me\AppData\Local`, `C:\Program Files`
	cases := map[string]bool{
		`C:\Users\me\AppData\Local\Programs\Pigeon\pigeon.exe`: true,
		`c:\users\ME\appdata\local\programs\pigeon\pigeon.exe`: true,
		`C:\Program Files\Pigeon\Pigeon\pigeon.exe`:            true,
		`C:\Users\me\Downloads\Pigeon\pigeon.exe`:              false, // portable zip
		`D:\Projects\Personal\Pigeon\bin\pigeon.exe`:           false, // dev build
	}
	for exe, want := range cases {
		if got := installedAt(exe, local, pf); got != want {
			t.Errorf("installedAt(%s) = %v, want %v", exe, got, want)
		}
	}
}
