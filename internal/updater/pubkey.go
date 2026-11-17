package updater

// PublicKey verifies release signatures (base64 Ed25519 public key). It is
// safe to commit. Generate the key pair with:
//
//	go run ./scripts/update-keys
//
// which writes the public key here and the private key to a file you add as
// the PIGEON_UPDATE_SIGNING_KEY GitHub secret. While empty, auto-update is
// disabled: Pigeon never installs anything it can't verify.
const PublicKey = "jXIY0lRJgxDTyWBU+qykNN8XPgXgd0T5SQViuHMk1AA="
