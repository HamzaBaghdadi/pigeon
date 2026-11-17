# Sign in with Microsoft

Pigeon signs in to Outlook.com, Hotmail, Live and Microsoft 365 accounts with
Microsoft's own sign-in page (OAuth 2.0 with PKCE). Pigeon never sees the
password, and Microsoft has turned off password sign-in for most of these
mailboxes anyway.

To offer the button, Pigeon needs an **app registration** in Microsoft Entra.
It's free and takes about five minutes. The registration is a *public client*:
there is no client secret, only an Application (client) ID. That ID is not
confidential (it appears in every sign-in URL), but like the Google client it is
kept out of the repository and embedded only in builds.

## 1. Register the app

1. Open the [Microsoft Entra admin center](https://entra.microsoft.com) and go to
   **Identity → Applications → App registrations → New registration**.
   - You need a directory (tenant) you control. If you only have a personal
     Microsoft account, sign in to the [Azure portal](https://portal.azure.com)
     once and create a free account; that gives you a default directory.
2. Fill in:
   - **Name:** `Pigeon`
   - **Supported account types:** *Accounts in any organizational directory and
     personal Microsoft accounts* (multitenant + personal). This is what lets
     both Outlook.com and work or school accounts sign in.
   - **Redirect URI:** platform **Public client/native (mobile & desktop)**,
     value `http://localhost`

     Microsoft ignores the port when matching `localhost`, so Pigeon's random
     sign-in port works with this single entry. Don't add a path.
3. Click **Register**, then copy the **Application (client) ID** from the
   overview page (it looks like `00000000-0000-0000-0000-000000000000`).

## 2. Add the mail permissions

Open **API permissions → Add a permission → Microsoft Graph → Delegated
permissions** and add:

| Permission | Why |
|---|---|
| `IMAP.AccessAsUser.All` | read and organize mail over IMAP |
| `SMTP.Send` | send mail |
| `offline_access` | keep syncing without signing in again |
| `openid`, `email`, `profile` | learn the account's address and name |

Pigeon requests the same scopes at sign-in, so personal accounts would work
without this step. Listing them here makes the consent screen and admin
approval for organizations show exactly what Pigeon asks for.

Do **not** click "Grant admin consent" unless you are the admin of an
organization whose users you want to pre-approve.

## 3. Try it locally

In PowerShell, from the repository root:

```powershell
$env:PIGEON_MICROSOFT_CLIENT_ID = "your-client-id"
go run ./scripts/embed-credentials
wails3 build
```

This writes the gitignored `services/release_credentials.go`. Open
`bin\pigeon.exe` and **Sign in with Microsoft** appears in **Add an account**.
Remove the file again when you're done:

```powershell
go run ./scripts/embed-credentials -clean
```

## 4. Ship it in releases

Add the repository secret `PIGEON_MICROSOFT_CLIENT_ID` (see
[releasing.md](releasing.md)). Release builds then show the button for everyone.
Without the secret the button is hidden and the release still builds.

## What people may run into

- **Work or school accounts: "Need admin approval".** Many organizations only
  let users approve apps from verified publishers, or none at all. Pigeon shows
  a message asking them to contact their IT admin. To reduce this, complete
  [publisher verification](https://learn.microsoft.com/entra/identity-platform/publisher-verification-overview)
  (needs a Microsoft AI Cloud Partner Program ID), which also removes the
  "unverified" label on the consent screen.
- **IMAP turned off for the mailbox.** Microsoft 365 admins can disable IMAP.
  Pigeon reports that sign-in worked but the mailbox refused IMAP access.
- **Sending blocked (SMTP AUTH disabled).** Microsoft 365 turns off
  authenticated SMTP by default in newer tenants. Reading mail works; sending
  shows a message asking the IT admin to enable SMTP AUTH for the mailbox.
- **Personal accounts** (Outlook.com, Hotmail, Live) work without any of the
  above.

## Security notes

- The redirect only reaches `localhost` on a random port, and only the first
  request with the right `state` value is accepted.
- PKCE (S256) ties the authorization code to this Pigeon instance.
- Tokens are stored in Windows Credential Manager. Microsoft access tokens can
  exceed its per-entry size limit; in that case Pigeon stores only the refresh
  token and fetches a fresh access token when it connects.
- Removing the account in Pigeon deletes its tokens. To revoke access entirely,
  visit [account.live.com/consent/Manage](https://account.live.com/consent/Manage)
  (personal) or [myapps.microsoft.com](https://myapps.microsoft.com) (work).
