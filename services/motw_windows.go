//go:build windows

package services

import "os"

// markDownloaded adds Windows' "Mark of the Web" (Zone.Identifier, Internet
// zone) to a file Pigeon saved from an email, just like browsers do. Windows
// SmartScreen, Office Protected View and other tools then treat it as
// untrusted content from the internet.
func markDownloaded(path string) {
	_ = os.WriteFile(path+":Zone.Identifier", []byte("[ZoneTransfer]\r\nZoneId=3\r\n"), 0o644)
}
