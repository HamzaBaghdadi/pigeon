// Package secrets stores account passwords and OAuth tokens in the operating
// system's credential store (Windows Credential Manager, macOS Keychain,
// Secret Service on Linux). Secrets are never written to Pigeon's own files.
package secrets

import (
	"errors"

	"github.com/zalando/go-keyring"
)

const service = "io.github.hamzabaghdadi.pigeon"

// ErrNotFound is returned when no secret exists for the given key.
var ErrNotFound = errors.New("secret not found")

// Set stores a secret under key (for example "account:<id>:refresh_token").
func Set(key, value string) error {
	return keyring.Set(service, key, value)
}

// Get returns the secret stored under key.
func Get(key string) (string, error) {
	v, err := keyring.Get(service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return v, err
}

// Delete removes the secret stored under key. Deleting a missing key is not an error.
func Delete(key string) error {
	err := keyring.Delete(service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
