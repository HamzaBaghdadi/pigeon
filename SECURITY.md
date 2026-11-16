# Security Policy

Pigeon handles people's email and credentials, so security reports are a top priority.

## Reporting a vulnerability

Please **do not open a public issue**. Report privately through
[GitHub Security Advisories](https://github.com/HamzaBaghdadi/pigeon/security/advisories/new).

Include steps to reproduce, the affected version and the impact you expect.
You should get a first response within a few days.

## Security model

- **Email HTML is untrusted.** It is sanitized in Go (`internal/sanitize`) and rendered in a
  sandboxed iframe without script permission and with a CSP that blocks all network requests
  except images the user explicitly allows.
- **Links never navigate the app.** They are validated (http, https, mailto only) and opened in
  the system browser.
- **Credentials live in the OS keychain** (`internal/secrets`), never in Pigeon's config or database.
- **The app shell has a strict CSP** in production builds (`internal/security`).

## Supported versions

Only the latest release receives security fixes while Pigeon is pre-1.0.
