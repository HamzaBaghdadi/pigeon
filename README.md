<p align="center">
  <img src="build/appicon.png" width="128" alt="Pigeon logo">
</p>

<h1 align="center">Pigeon</h1>

<p align="center">
  A secure, open-source email client with a UI that gets out of your way.<br>
  All your accounts, one fast inbox. Built with Go, Wails v3 and Next.js.
</p>

---

> **Status: early development.** Reading mail works: link IMAP accounts (Gmail, iCloud, Yahoo, Fastmail and others), sync, search, archive, delete. Gmail works with Sign in with Google (no app password needed). Sending and Microsoft sign-in are next.

## Why Pigeon

- **Every account in one place.** Gmail, Outlook and any IMAP/SMTP provider, with a unified inbox that keeps each account's color.
- **Private by default.** Remote images (tracking pixels) are blocked until you allow them. No telemetry.
- **Secure email rendering.** Every email is sanitized in Go and then rendered in a sandboxed, script-free iframe with its own strict Content Security Policy.
- **Secrets stay in your OS keychain.** Passwords and OAuth tokens go to Windows Credential Manager, macOS Keychain or Secret Service, never to Pigeon's own files.
- **Keyboard-first.** `J`/`K` to move, `E` to archive, `S` to star, `U` to toggle read, `Z` to undo, `/` or `Ctrl+K` to search, `Ctrl+,` for settings.
- **Light and dark.** Follows your system theme by default, or pick Light or Dark in Settings. Pane widths are adjustable and remembered.
- **Fast and offline.** Mail is cached in a local SQLite database with full-text search, so the inbox opens instantly and search works without a connection.
- **Small.** Native webview instead of a bundled browser.

## Platforms

| Platform | Status |
|---|---|
| Windows 10/11 | 🟢 Primary target |
| macOS | 🟡 Planned |
| Linux | 🟡 Planned |

## Development

### Requirements

- [Go](https://go.dev/dl/) 1.26+
- [Node.js](https://nodejs.org/) 20+
- Wails v3 CLI: `go install github.com/wailsapp/wails/v3/cmd/wails3@latest`
- Windows: WebView2 runtime (preinstalled on Windows 11)

Check your setup with `wails3 doctor`.

### Run in dev mode

```bash
wails3 dev
```

This starts the Next.js dev server and the Go app with hot reload.

### Build a release binary

```bash
wails3 build
```

The binary is written to `bin/`. For an installer, use `wails3 package`.

### Tests

```bash
go test ./...
cd frontend && npm run typecheck && npm run lint
```

## Project layout

```
main.go                 App entry point: window, services, asset server
services/               Go APIs exposed to the frontend (Wails bindings)
internal/mail/          Core email types
internal/store/         Local SQLite mail database with full-text search
internal/imapsync/      IMAP sync engine (folders, messages, flags, moves)
internal/mailparse/     MIME parsing (charsets, multipart, attachments)
internal/providers/     Server settings for common providers
internal/oauth/         Sign in with Google (browser flow, PKCE, token refresh)
internal/settings/      User preferences (theme)
internal/sanitize/      HTML email sanitizer (first security layer)
internal/secrets/       OS keychain storage for passwords and tokens
internal/security/      Security headers for the embedded UI
frontend/               Next.js app (static export, embedded in the binary)
frontend/bindings/      Generated TypeScript bindings (wails3 generate bindings)
build/                  Icons, platform configs, installers
```

## Design

Pigeon's palette comes from its logo:

| Token | Hex | Used for |
|---|---|---|
| Royal blue | `#2B4FD8` | Primary actions, selection |
| Electric blue | `#1A73F0` | Gradient start |
| Cyan | `#22D3EE` | Highlights |
| Violet | `#6D3AE0` | Gradient end, accents |
| Periwinkle | `#C7D0F5` | Soft surfaces |
| Deep navy | `#1E2350` | Text |

## Roadmap

- [x] IMAP sync into a local SQLite store with full-text search
- [ ] Instant new-mail push (IMAP IDLE) and older-mail paging
- [ ] SMTP sending and a composer
- [x] Sign in with Google (OAuth 2.0 + PKCE, XOAUTH2). [Setup guide](docs/google-sign-in.md)
- [ ] Sign in with Microsoft (Outlook.com, Microsoft 365)
- [x] Unified inbox actions: archive, delete, star, read/unread, undo
- [ ] Move to any folder, snooze
- [ ] Notifications and system tray
- [ ] Encrypted local cache
- [ ] OpenPGP / S/MIME
- [ ] macOS and Linux builds

## Releases

Download the latest installer from [GitHub Releases](https://github.com/HamzaBaghdadi/pigeon/releases). Release builds include Sign in with Google out of the box. Maintainers: see [docs/releasing.md](docs/releasing.md).

## Security

Found a vulnerability? Please report it privately. See [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
