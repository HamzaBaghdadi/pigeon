// Command sign-release signs a release file (SHA256SUMS.txt) with the update
// key from PIGEON_UPDATE_SIGNING_KEY, writing <file>.sig. Used by CI.
//
//	go run ./scripts/sign-release dist/SHA256SUMS.txt
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/HamzaBaghdadi/pigeon/internal/updater"
)

func main() {
	if len(os.Args) != 2 {
		fail(fmt.Errorf("usage: sign-release <file>"))
	}
	raw := strings.TrimSpace(os.Getenv("PIGEON_UPDATE_SIGNING_KEY"))
	if raw == "" {
		fail(fmt.Errorf("PIGEON_UPDATE_SIGNING_KEY is not set"))
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(key) != ed25519.PrivateKeySize {
		fail(fmt.Errorf("PIGEON_UPDATE_SIGNING_KEY isn't a valid key (create one with scripts/update-keys)"))
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fail(err)
	}
	sig := ed25519.Sign(ed25519.PrivateKey(key), data)
	sigB64 := base64.StdEncoding.EncodeToString(sig)
	// Refuse to publish a release that installed copies couldn't verify
	// (e.g. the secret doesn't match the public key compiled into the app).
	if err := updater.Verify(updater.PublicKey, data, []byte(sigB64)); err != nil {
		fail(fmt.Errorf("signature doesn't verify against internal/updater/pubkey.go; check the PIGEON_UPDATE_SIGNING_KEY secret"))
	}
	if err := os.WriteFile(os.Args[1]+".sig", []byte(sigB64+"\n"), 0o644); err != nil {
		fail(err)
	}
	fmt.Println("signed and verified", os.Args[1])
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "sign-release:", err)
	os.Exit(1)
}
