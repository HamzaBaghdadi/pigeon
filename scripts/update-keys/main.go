// Command update-keys creates the Ed25519 key pair that signs Pigeon releases.
//
//	go run ./scripts/update-keys [-out path]
//
// The public key is written into internal/updater/pubkey.go (commit it).
// The private key is written to a file OUTSIDE the repository (default:
// your home folder) and is never printed. Add its contents as the
// PIGEON_UPDATE_SIGNING_KEY GitHub secret, then store the file somewhere
// safe (a password manager) and delete it.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const pubkeyFile = "internal/updater/pubkey.go"

func main() {
	home, _ := os.UserHomeDir()
	out := flag.String("out", filepath.Join(home, "pigeon-update-signing-key.txt"), "where to write the private key")
	force := flag.Bool("force", false, "replace an existing public key (old releases' updates will stop verifying)")
	flag.Parse()

	src, err := os.ReadFile(pubkeyFile)
	if err != nil {
		fail(fmt.Errorf("run this from the repository root: %w", err))
	}
	re := regexp.MustCompile(`const PublicKey = "([^"]*)"`)
	m := re.FindSubmatch(src)
	if m == nil {
		fail(fmt.Errorf("couldn't find PublicKey in %s", pubkeyFile))
	}
	if len(m[1]) > 0 && !*force {
		fail(fmt.Errorf("a public key is already set. Use -force only if you've lost the private key (installed copies won't accept updates signed with a new key)"))
	}
	if _, err := os.Stat(*out); err == nil {
		fail(fmt.Errorf("%s already exists; refusing to overwrite a private key", *out))
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(*out, []byte(base64.StdEncoding.EncodeToString(priv)+"\n"), 0o600); err != nil {
		fail(err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	src = re.ReplaceAll(src, []byte(`const PublicKey = "`+pubB64+`"`))
	if err := os.WriteFile(pubkeyFile, src, 0o644); err != nil {
		fail(err)
	}

	fmt.Println("Public key written to", pubkeyFile+":", pubB64)
	fmt.Println("Private key written to", *out, "(not shown).")
	fmt.Println()
	fmt.Println("Next:")
	fmt.Println("  1. Add the file's contents as the GitHub secret PIGEON_UPDATE_SIGNING_KEY.")
	fmt.Println("  2. Save the file in a password manager, then delete it.")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "update-keys:", err)
	os.Exit(1)
}
