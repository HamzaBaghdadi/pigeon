# Releasing Pigeon

Releases are built by GitHub Actions ([`.github/workflows/release.yml`](../.github/workflows/release.yml))
when you push a version tag. Each release contains:

- `Pigeon-vX.Y.Z-windows-setup.exe`: installer (per-user, no admin rights needed)
- `Pigeon-vX.Y.Z-windows-portable.zip`: the app without an installer
- `SHA256SUMS.txt` and `SHA256SUMS.txt.sig`: checksums, signed with the update key
- Signed build provenance, so anyone can verify the files were built from this
  repository: `gh attestation verify <file> --repo HamzaBaghdadi/pigeon`

## One-time setup: Sign in with Google for release users

Release builds include Pigeon's own Google OAuth client, so people who download
the app just click **Sign in with Google**. They never see the client setup
screen. The client is stored only in GitHub's encrypted secrets; it is never
committed and never printed in logs.

1. In Google Cloud, use the project from [google-sign-in.md](google-sign-in.md)
   and make sure the app is **published** (Audience → Publish app).
2. Create a client just for releases: **Clients → Create client → Desktop app**,
   name it `Pigeon Releases`. Keeping it separate from your personal client
   means you can rotate either one without affecting the other.
3. In GitHub, open the repository → **Settings → Secrets and variables →
   Actions → New repository secret**, and add:

   | Name | Value |
   |---|---|
   | `PIGEON_GOOGLE_CLIENT_ID` | the client ID (ends with `.apps.googleusercontent.com`) |
   | `PIGEON_GOOGLE_CLIENT_SECRET` | the client secret |

If either secret is missing, the release job fails instead of shipping an app
whose Google sign-in doesn't work.

### What users see until Google verifies the app

- Up to **100 users** can sign in with Google (Google's limit for unverified
  apps that read Gmail).
- During sign-in, Google shows **"Google hasn't verified this app"**; users
  click **Advanced → Go to Pigeon**.

To remove both, submit the app for verification in Google Auth Platform
(**Verification Center**). For the Gmail scope this requires a privacy policy,
a homepage, a demo video and a yearly third-party security assessment (CASA).
Do this once Pigeon is ready for a wider audience. Password and app-password
sign-in keep working for everyone in the meantime.

### Is it safe to ship the client secret in the app?

Yes. Google treats desktop-app client secrets as public: anyone can extract
them from any desktop app. Security comes from PKCE, the loopback redirect and
the user approving access on Google's own page. The client can't be used to
read anyone's mail without that approval. We still keep it out of the
repository so it isn't copied into forks and other projects, and so it can be
rotated without a code change.

## One-time setup: Sign in with Microsoft for release users

Register the app as described in [microsoft-sign-in.md](microsoft-sign-in.md),
then add one more repository secret:

| Name | Value |
|---|---|
| `PIGEON_MICROSOFT_CLIENT_ID` | the Application (client) ID |

There is no Microsoft client secret: the registration is a public client. If
this secret is missing, releases still build and simply hide the button.

## One-time setup: signed auto-updates

Installed copies of Pigeon check GitHub Releases every 6 hours (and 15 seconds
after launch), download new versions in the background and offer **Restart to
update**. They only install an update whose `SHA256SUMS.txt` is signed with
your update key, and only if the installer matches the signed checksum. A
swapped or corrupted download, or a release file uploaded by someone who can
edit releases but not run your workflow, is rejected. The private key lives
only in GitHub's encrypted secrets, so anyone who can change your workflows
could misuse it: keep two-factor authentication on your GitHub account and
review workflow changes in pull requests.

1. From the repository root, run:

   ```bash
   go run ./scripts/update-keys
   ```

   This writes the **public** key into `internal/updater/pubkey.go` (commit
   it) and the **private** key to `pigeon-update-signing-key.txt` in your home
   folder. The private key is never printed.
2. Add the file's contents as the repository secret
   `PIGEON_UPDATE_SIGNING_KEY`.
3. Save the file in a password manager, then delete it. **If you lose it,**
   copies already installed can't verify updates from a new key, and users will
   have to download one new version manually.

The release job refuses to run without a public key in the code and the
signing secret, so you can't accidentally ship an update nobody can install.

Development builds (`dev` version) and portable copies never update
themselves. Portable copies show **Download** instead of **Restart to update**.

## Cutting a release

1. Update `version` in [`build/config.yml`](../build/config.yml), then run
   `wails3 task common:update:build-assets` to refresh the Windows version info.
2. Commit, then tag and push:

   ```bash
   git tag v0.2.0
   git push origin v0.2.0
   ```

3. Wait for the **Release** workflow to finish. It creates a **draft** release.
4. Review the files and notes on GitHub, then click **Publish release**.

## Code signing

Builds are not code-signed yet, so Windows SmartScreen shows "Windows protected
your PC" on first run (**More info → Run anyway**). Signing needs a code-signing
certificate (for example Azure Trusted Signing). The Windows Taskfile already
has a `sign` task for when one is available.

## If a secret leaks

1. In Google Cloud → Clients, delete the leaked client and create a new one.
   (The Microsoft client ID isn't a secret, so there's nothing to rotate there.)
2. Update the GitHub secrets and publish a new release.
3. Existing users sign in again with **Reconnect** in Settings → Accounts.
