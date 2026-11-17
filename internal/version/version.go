// Package version reports which Pigeon build is running.
package version

import "strings"

// Version is "dev" for local builds. Release builds set it from the git tag
// (e.g. "0.2.0") by generating the gitignored release_version.go in this
// package (see scripts/embed-credentials).
var Version = "dev"

// IsRelease reports whether this is an official, versioned build. Only those
// check for updates.
func IsRelease() bool {
	return Version != "" && Version != "dev" && !strings.Contains(Version, "-dev")
}
