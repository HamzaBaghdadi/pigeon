package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	pmail "github.com/HamzaBaghdadi/pigeon/internal/mail"
	"github.com/HamzaBaghdadi/pigeon/internal/imapsync"
	"github.com/HamzaBaghdadi/pigeon/internal/providers"
	"github.com/HamzaBaghdadi/pigeon/internal/secrets"
	"github.com/HamzaBaghdadi/pigeon/internal/store"
)

// AccountService links and removes accounts and controls sync.
type AccountService struct {
	core *Core
}

func NewAccountService(core *Core) *AccountService { return &AccountService{core: core} }

// ServiceStartup starts background sync when the app starts.
func (s *AccountService) ServiceStartup(context.Context, application.ServiceOptions) error {
	s.core.Start()
	return nil
}

// ServiceShutdown stops background sync.
func (s *AccountService) ServiceShutdown() error {
	s.core.Stop()
	return nil
}

// DetectSettings suggests server settings for an email address.
func (s *AccountService) DetectSettings(email string) providers.Settings {
	return providers.Detect(email)
}

// AddAccountInput is what the Add account form submits.
type AddAccountInput struct {
	Name         string `json:"name"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	Username     string `json:"username"` // optional; defaults to the email address
	IMAPHost     string `json:"imapHost"`
	IMAPPort     int    `json:"imapPort"`
	IMAPSecurity string `json:"imapSecurity"`
	SMTPHost     string `json:"smtpHost"`
	SMTPPort     int    `json:"smtpPort"`
	SMTPSecurity string `json:"smtpSecurity"`
}

var accountColors = []string{"#2B4FD8", "#7C4DFF", "#06B6D4", "#E5484D", "#F5A524", "#30A46C", "#D6409F"}

// AddAccount checks the settings by signing in, then saves the account
// (password to the OS keychain) and starts syncing it.
func (s *AccountService) AddAccount(in AddAccountInput) (pmail.Account, error) {
	in.Email = strings.TrimSpace(in.Email)
	in.IMAPHost = strings.TrimSpace(in.IMAPHost)
	in.SMTPHost = strings.TrimSpace(in.SMTPHost)
	if _, err := mail.ParseAddress(in.Email); err != nil || !strings.Contains(in.Email, "@") {
		return pmail.Account{}, errors.New("enter a valid email address")
	}
	if in.Password == "" {
		return pmail.Account{}, errors.New("enter your password")
	}
	if in.IMAPHost == "" || in.IMAPPort <= 0 || in.IMAPPort > 65535 {
		return pmail.Account{}, errors.New("enter the incoming (IMAP) server and port")
	}
	if in.IMAPSecurity != string(imapsync.SecurityTLS) && in.IMAPSecurity != string(imapsync.SecurityStartTLS) {
		in.IMAPSecurity = string(imapsync.SecurityTLS)
	}
	username := strings.TrimSpace(in.Username)
	if username == "" {
		username = in.Email
	}

	existing, err := s.core.store.ListAccounts()
	if err != nil {
		return pmail.Account{}, err
	}
	realCount := 0
	for _, a := range existing {
		if !a.Demo && strings.EqualFold(a.Email, in.Email) {
			return pmail.Account{}, fmt.Errorf("%s is already added", in.Email)
		}
		if !a.Demo {
			realCount++
		}
	}

	if err := imapsync.Verify(imapsync.Config{
		Host: in.IMAPHost, Port: in.IMAPPort, Security: imapsync.Security(in.IMAPSecurity),
		Username: username, Password: in.Password,
	}); err != nil {
		return pmail.Account{}, err
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = in.Email[:strings.Index(in.Email, "@")]
	}
	a := store.Account{
		Account: pmail.Account{
			ID: newID(), Name: name, Email: in.Email,
			Provider: providers.Detect(in.Email).Provider,
			Color:    accountColors[realCount%len(accountColors)],
		},
		Username: username,
		IMAPHost: in.IMAPHost, IMAPPort: in.IMAPPort, IMAPSecurity: in.IMAPSecurity,
		SMTPHost: in.SMTPHost, SMTPPort: in.SMTPPort, SMTPSecurity: in.SMTPSecurity,
	}
	if err := secrets.Set(passwordKey(a.ID), in.Password); err != nil {
		return pmail.Account{}, fmt.Errorf("couldn't save the password to Windows Credential Manager: %w", err)
	}
	if err := s.core.store.SaveAccount(a); err != nil {
		_ = secrets.Delete(passwordKey(a.ID))
		return pmail.Account{}, err
	}
	// The first real account replaces the sample mail.
	if realCount == 0 {
		_ = s.core.store.DeleteDemoAccounts()
	}
	emit(EventMailChanged, "")
	s.core.SyncSoon()
	return a.Account, nil
}

// RemoveAccount forgets an account: local mail, settings and saved password.
// Nothing is deleted on the server.
func (s *AccountService) RemoveAccount(id string) error {
	s.core.engine.Disconnect(id)
	if err := s.core.store.DeleteAccount(id); err != nil {
		return err
	}
	_ = secrets.Delete(passwordKey(id))
	_ = secrets.Delete(oauthKey(id))
	s.core.google.forget(id)
	emit(EventMailChanged, "")
	return nil
}

// LoadSampleData fills Pigeon with sample accounts and mail to explore the app.
func (s *AccountService) LoadSampleData() error {
	if err := loadDemo(s.core.store); err != nil {
		return err
	}
	emit(EventMailChanged, "")
	return nil
}

// RemoveSampleData deletes the sample accounts and mail.
func (s *AccountService) RemoveSampleData() error {
	if err := s.core.store.DeleteDemoAccounts(); err != nil {
		return err
	}
	emit(EventMailChanged, "")
	return nil
}

// SyncNow syncs all accounts right away.
func (s *AccountService) SyncNow() { s.core.SyncSoon() }

// GetSyncStatus returns the current background sync status.
func (s *AccountService) GetSyncStatus() SyncStatus { return s.core.Status() }

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
