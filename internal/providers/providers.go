// Package providers knows the server settings of common email providers so
// most people only need to type their address and password.
package providers

import (
	"strings"

	"github.com/HamzaBaghdadi/pigeon/internal/mail"
)

// Settings are suggested connection settings for an address.
type Settings struct {
	Provider     mail.Provider `json:"provider"`
	Label        string        `json:"label"` // e.g. "Gmail"
	IMAPHost     string        `json:"imapHost"`
	IMAPPort     int           `json:"imapPort"`
	IMAPSecurity string        `json:"imapSecurity"`
	SMTPHost     string        `json:"smtpHost"`
	SMTPPort     int           `json:"smtpPort"`
	SMTPSecurity string        `json:"smtpSecurity"`
	// PasswordHint explains which password to use (often an app password).
	PasswordHint string `json:"passwordHint"`
	// HelpURL points to where the user creates that password.
	HelpURL string `json:"helpUrl"`
	// Warning is shown when password sign-in is known not to work.
	Warning string `json:"warning"`
	// Known is false when the settings are only a guess from the domain.
	Known bool `json:"known"`
}

type preset struct {
	Settings
	domains []string
}

var presets = []preset{
	{domains: []string{"gmail.com", "googlemail.com"}, Settings: Settings{
		Provider: mail.ProviderGmail, Label: "Gmail",
		IMAPHost: "imap.gmail.com", IMAPPort: 993, IMAPSecurity: "tls",
		SMTPHost: "smtp.gmail.com", SMTPPort: 465, SMTPSecurity: "tls",
		PasswordHint: "Use a Google app password, not your normal password. It needs 2-Step Verification turned on.",
		HelpURL:      "https://myaccount.google.com/apppasswords",
	}},
	{domains: []string{
		"outlook.com", "hotmail.com", "live.com", "msn.com", "passport.com",
		"outlook.fr", "outlook.de", "outlook.es", "outlook.it", "outlook.sa", "outlook.co.uk",
		"hotmail.co.uk", "hotmail.fr", "hotmail.de", "hotmail.es", "hotmail.it",
		"live.co.uk", "live.fr", "live.de", "live.nl", "live.ca",
	}, Settings: Settings{
		Provider: mail.ProviderOutlook, Label: "Outlook.com",
		IMAPHost: "outlook.office365.com", IMAPPort: 993, IMAPSecurity: "tls",
		SMTPHost: "smtp-mail.outlook.com", SMTPPort: 587, SMTPSecurity: "starttls",
		// The UI points to Sign in with Microsoft when this build offers it.
		Warning: "Microsoft has turned off password sign-in for most Outlook.com accounts.",
	}},
	{domains: []string{"yahoo.com", "ymail.com", "yahoo.co.uk", "yahoo.fr", "rocketmail.com"}, Settings: Settings{
		Provider: mail.ProviderIMAP, Label: "Yahoo Mail",
		IMAPHost: "imap.mail.yahoo.com", IMAPPort: 993, IMAPSecurity: "tls",
		SMTPHost: "smtp.mail.yahoo.com", SMTPPort: 465, SMTPSecurity: "tls",
		PasswordHint: "Use a Yahoo app password, generated in your Yahoo account security settings.",
		HelpURL:      "https://login.yahoo.com/account/security",
	}},
	{domains: []string{"icloud.com", "me.com", "mac.com"}, Settings: Settings{
		Provider: mail.ProviderIMAP, Label: "iCloud Mail",
		IMAPHost: "imap.mail.me.com", IMAPPort: 993, IMAPSecurity: "tls",
		SMTPHost: "smtp.mail.me.com", SMTPPort: 587, SMTPSecurity: "starttls",
		PasswordHint: "Use an app-specific password from your Apple Account.",
		HelpURL:      "https://account.apple.com/account/manage",
	}},
	{domains: []string{"fastmail.com", "fastmail.fm"}, Settings: Settings{
		Provider: mail.ProviderIMAP, Label: "Fastmail",
		IMAPHost: "imap.fastmail.com", IMAPPort: 993, IMAPSecurity: "tls",
		SMTPHost: "smtp.fastmail.com", SMTPPort: 465, SMTPSecurity: "tls",
		PasswordHint: "Use a Fastmail app password.",
		HelpURL:      "https://app.fastmail.com/settings/security/apppasswords",
	}},
	{domains: []string{"zoho.com", "zohomail.com"}, Settings: Settings{
		Provider: mail.ProviderIMAP, Label: "Zoho Mail",
		IMAPHost: "imap.zoho.com", IMAPPort: 993, IMAPSecurity: "tls",
		SMTPHost: "smtp.zoho.com", SMTPPort: 465, SMTPSecurity: "tls",
	}},
	{domains: []string{"aol.com"}, Settings: Settings{
		Provider: mail.ProviderIMAP, Label: "AOL Mail",
		IMAPHost: "imap.aol.com", IMAPPort: 993, IMAPSecurity: "tls",
		SMTPHost: "smtp.aol.com", SMTPPort: 465, SMTPSecurity: "tls",
		PasswordHint: "Use an AOL app password.",
	}},
	{domains: []string{"gmx.com", "gmx.net", "gmx.de"}, Settings: Settings{
		Provider: mail.ProviderIMAP, Label: "GMX",
		IMAPHost: "imap.gmx.com", IMAPPort: 993, IMAPSecurity: "tls",
		SMTPHost: "mail.gmx.com", SMTPPort: 587, SMTPSecurity: "starttls",
	}},
	{domains: []string{"proton.me", "protonmail.com", "pm.me"}, Settings: Settings{
		Provider: mail.ProviderIMAP, Label: "Proton Mail",
		Warning: "Proton Mail only allows IMAP through Proton Mail Bridge, which Pigeon doesn't support yet.",
	}},
}

// Detect suggests settings for an email address.
func Detect(email string) Settings {
	domain := strings.ToLower(strings.TrimSpace(email))
	if i := strings.LastIndex(domain, "@"); i >= 0 {
		domain = domain[i+1:]
	}
	for _, p := range presets {
		for _, d := range p.domains {
			if d == domain {
				s := p.Settings
				s.Known = true
				return s
			}
		}
	}
	if domain == "" || !strings.Contains(domain, ".") {
		return Settings{Provider: mail.ProviderIMAP, IMAPPort: 993, IMAPSecurity: "tls", SMTPPort: 465, SMTPSecurity: "tls"}
	}
	return Settings{
		Provider: mail.ProviderIMAP,
		IMAPHost: "imap." + domain, IMAPPort: 993, IMAPSecurity: "tls",
		SMTPHost: "smtp." + domain, SMTPPort: 465, SMTPSecurity: "tls",
	}
}
