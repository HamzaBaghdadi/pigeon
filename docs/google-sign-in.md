# Setting up Sign in with Google

Gmail no longer accepts your normal Google password from email apps. Pigeon
uses **Sign in with Google** instead: Google's own sign-in page opens in your
browser, you approve access, and Pigeon receives a revocable key. Pigeon never
sees your password.

Google requires every app that reads Gmail to be registered in Google Cloud.
Until Pigeon ships official builds with a built-in registration, each user
creates their own. It's free and takes about 10 minutes. You do it once.

## 1. Create a project

Open <https://console.cloud.google.com/projectcreate>, name it (for example
**Pigeon**) and click **Create**. Make sure the new project is selected at the
top of the page for the next steps.

## 2. Enable the Gmail API

Open <https://console.cloud.google.com/apis/library/gmail.googleapis.com> and
click **Enable**.

## 3. Configure Google Auth Platform

Open <https://console.cloud.google.com/auth/overview> and click **Get started**:

1. **App information:** app name `Pigeon`, your email as the support email.
2. **Audience:** choose **External**.
3. **Contact information:** your email.
4. Accept the policy and click **Create**.

Then, under **Audience**:

- Add your Gmail address under **Test users**.
- Click **Publish app** and confirm.

Publishing matters: while an app is in *Testing*, Google expires the sign-in
every 7 days and you'd have to reconnect weekly. A published but unverified app
works for up to 100 users, with no expiry. Google shows a "Google hasn't
verified this app" screen during sign-in; click **Advanced → Go to Pigeon
(unsafe)**. That warning is about the registration being unreviewed (it's your
own), not about your account.

## 4. Create the client

Open <https://console.cloud.google.com/auth/clients> and click **Create client**:

- **Application type:** Desktop app
- **Name:** Pigeon

Click **Create**. Copy the **Client ID** and **Client secret**.

## 5. Add it to Pigeon

In Pigeon, choose **Add account → Sign in with Google**. The first time, it
asks for the Client ID and Client secret. Paste them and click **Save and sign
in**. Your browser opens Google's sign-in page; approve access and Pigeon
starts syncing.

You can change the client later in **Settings → Accounts → Sign in with
Google**.

## Where things are stored

| What | Where |
|---|---|
| Client ID | `%AppData%\Pigeon\settings.json` |
| Client secret | Windows Credential Manager |
| Your account's sign-in key (refresh token) | Windows Credential Manager |

To revoke Pigeon's access at any time, go to
<https://myaccount.google.com/connections>, or remove the account in Pigeon.

## Troubleshooting

- **"Access blocked: Pigeon has not completed the Google verification process"**:
  your address isn't listed as a test user (step 3).
- **"Gmail refused IMAP access"**: in Gmail on the web, open
  **Settings → See all settings → Forwarding and POP/IMAP** and make sure IMAP is
  enabled.
- **"Sign-in expired"**: the app is still in *Testing* (7-day expiry) or access
  was revoked. Publish the app (step 3), then click **Reconnect** in
  **Settings → Accounts**.

## For release builds

People who download Pigeon from GitHub Releases don't need any of this: release
builds include a built-in Google client from GitHub secrets. See
[releasing.md](releasing.md).
