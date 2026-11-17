package services

import (
	pmail "github.com/HamzaBaghdadi/pigeon/internal/mail"
	"github.com/HamzaBaghdadi/pigeon/internal/oauth"
	"github.com/HamzaBaghdadi/pigeon/internal/store"
)

// MicrosoftClientID is the app's Microsoft registration (a public client, so
// there is no secret). Like the Google client it is empty in source and filled
// in for official builds by scripts/embed-credentials.
var MicrosoftClientID string

func newMicrosoftAuth() *oauthAuth {
	return newOAuthAuth(oauth.Microsoft, func() oauth.Client { return oauth.Client{ID: MicrosoftClientID} })
}

// MicrosoftSetup describes whether Sign in with Microsoft is available.
type MicrosoftSetup struct {
	Configured bool `json:"configured"`
}

// GetMicrosoftSetup reports whether this build can sign in with Microsoft.
func (s *AccountService) GetMicrosoftSetup() MicrosoftSetup {
	return MicrosoftSetup{Configured: s.core.microsoft.configured()}
}

// SignInWithMicrosoft opens Microsoft's sign-in page in the browser and, once
// the user approves, adds the Outlook.com or Microsoft 365 account.
func (s *AccountService) SignInWithMicrosoft(loginHint, name string) (pmail.Account, error) {
	return s.addOAuthAccount(oauthAccountSpec{
		auth:        s.core.microsoft,
		authKind:    store.AuthMicrosoft,
		provider:    pmail.ProviderOutlook,
		servers:     microsoftServers(),
		defaultName: "Outlook",
		imapRefused: "Microsoft signed you in, but the mailbox refused IMAP access. For work or school accounts, your IT admin may need to turn on IMAP",
	}, loginHint, name)
}

// microsoftServers works for both Outlook.com and Microsoft 365 mailboxes.
func microsoftServers() store.Account {
	return store.Account{
		IMAPHost: "outlook.office365.com", IMAPPort: 993, IMAPSecurity: "tls",
		SMTPHost: "smtp.office365.com", SMTPPort: 587, SMTPSecurity: "starttls",
	}
}
