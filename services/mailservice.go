// Package services contains the Go APIs exposed to the frontend via Wails bindings.
package services

import (
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/HamzaBaghdadi/pigeon/internal/mail"
	"github.com/HamzaBaghdadi/pigeon/internal/sanitize"
	"github.com/HamzaBaghdadi/pigeon/internal/store"
)

// MailService is the frontend's entry point for reading and organizing mail.
// Reads come from the local store; changes go to the store first (instant UI)
// and then to the server.
type MailService struct {
	core *Core
}

func NewMailService(core *Core) *MailService { return &MailService{core: core} }

// ListAccounts returns every linked account.
func (s *MailService) ListAccounts() ([]mail.Account, error) {
	rows, err := s.core.store.ListAccounts()
	out := make([]mail.Account, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Account)
	}
	return out, err
}

// ListFolders returns the folders of one account, or of all accounts if accountID is empty.
func (s *MailService) ListFolders(accountID string) ([]mail.Folder, error) {
	rows, err := s.core.store.ListFolders(accountID)
	out := make([]mail.Folder, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Folder)
	}
	return out, err
}

// ListMessages returns messages for a folder role across one account (or all
// accounts when accountID is empty), newest first. query runs a full-text search.
func (s *MailService) ListMessages(accountID string, role mail.FolderRole, query string) ([]mail.MessageSummary, error) {
	return s.core.store.ListMessages(accountID, role, query, 1000)
}

// GetMessage returns a message body, downloading it first if needed, sanitized for display.
func (s *MailService) GetMessage(id string) (mail.MessageBody, error) {
	m, err := s.core.store.GetMessage(id)
	if err != nil {
		return mail.MessageBody{}, err
	}
	// Download the body if needed, or re-read messages stored before Pigeon
	// indexed attachments and embedded images.
	needsParts := m.BodyFetched && !m.PartsIndexed && (m.HasAttachments || strings.Contains(strings.ToLower(m.BodyHTML), "cid:"))
	if !m.BodyFetched || needsParts {
		a, err := s.core.store.GetAccount(m.AccountID)
		if err != nil {
			return mail.MessageBody{}, err
		}
		if !a.Demo {
			if err := s.core.engine.FetchBody(a, m); err != nil && !m.BodyFetched {
				return mail.MessageBody{}, fmt.Errorf("couldn't download this message: %w", err)
			}
			if m, err = s.core.store.GetMessage(id); err != nil {
				return mail.MessageBody{}, err
			}
		}
	}
	raw := m.BodyHTML
	if strings.TrimSpace(raw) == "" {
		raw = textToHTML(m.BodyText)
	}
	if images, err := s.core.store.InlineImages(id); err == nil {
		raw = inlineImages(raw, images)
	}
	clean := sanitize.HTML(raw)
	body := mail.MessageBody{ID: id, HTML: clean.HTML, Text: m.BodyText, RemoteImages: clean.RemoteImages, Attachments: []mail.Attachment{}}
	if parts, err := s.core.store.ListParts(id); err == nil {
		for _, p := range parts {
			if !p.Inline {
				body.Attachments = append(body.Attachments, p)
			}
		}
	}
	return body, nil
}

// SetRead marks a message read or unread.
func (s *MailService) SetRead(id string, read bool) error {
	unread := !read
	return s.changeFlag(id, func() error { return s.core.store.SetFlags(id, &unread, nil) },
		func() error { was := read; return s.core.store.SetFlags(id, &was, nil) },
		imap.FlagSeen, read)
}

// SetStarred stars or unstars a message.
func (s *MailService) SetStarred(id string, starred bool) error {
	return s.changeFlag(id, func() error { return s.core.store.SetFlags(id, nil, &starred) },
		func() error { was := !starred; return s.core.store.SetFlags(id, nil, &was) },
		imap.FlagFlagged, starred)
}

func (s *MailService) changeFlag(id string, apply, revert func() error, flag imap.Flag, on bool) error {
	m, a, err := s.load(id)
	if err != nil {
		return err
	}
	if err := apply(); err != nil {
		return err
	}
	if a.Demo {
		return nil
	}
	if err := s.core.engine.SetFlag(a, m, flag, on); err != nil {
		_ = revert()
		return fmt.Errorf("couldn't update the message on the server: %w", err)
	}
	return nil
}

// MoveResult says where a moved message went, so the UI can undo.
type MoveResult struct {
	// ID is the message's new ID, or "" if it isn't known until the next sync.
	ID string `json:"id"`
	// From is the role of the folder it came from.
	From mail.FolderRole `json:"from"`
	// FromFolder is the ID of the folder it came from (for undo).
	FromFolder string `json:"fromFolder"`
}

// Move moves a message to the account's folder with the given role.
func (s *MailService) Move(id string, role mail.FolderRole) (MoveResult, error) {
	m, a, err := s.load(id)
	if err != nil {
		return MoveResult{}, err
	}
	from, err := s.core.store.GetFolder(m.FolderID)
	if err != nil {
		return MoveResult{}, err
	}
	to, err := s.core.store.FolderByRole(a.ID, role)
	if errors.Is(err, store.ErrNotFound) {
		return MoveResult{}, fmt.Errorf("%s has no %s folder", a.Name, role)
	} else if err != nil {
		return MoveResult{}, err
	}
	return s.moveTo(m, a, from, to)
}

// MoveToFolder moves a message to a specific folder of its account.
func (s *MailService) MoveToFolder(id, folderID string) (MoveResult, error) {
	m, a, err := s.load(id)
	if err != nil {
		return MoveResult{}, err
	}
	from, err := s.core.store.GetFolder(m.FolderID)
	if err != nil {
		return MoveResult{}, err
	}
	to, err := s.core.store.GetFolder(folderID)
	if err != nil || to.AccountID != a.ID {
		return MoveResult{}, errors.New("that folder belongs to a different account")
	}
	if to.ID == from.ID {
		return MoveResult{ID: m.ID, From: from.Role, FromFolder: from.ID}, nil
	}
	return s.moveTo(m, a, from, to)
}

func (s *MailService) moveTo(m store.MessageRow, a store.Account, from, to store.FolderRow) (MoveResult, error) {
	id := m.ID
	var err error
	res := MoveResult{From: from.Role, FromFolder: from.ID}
	if a.Demo {
		res.ID, err = s.core.store.MoveLocal(id, to.ID, uint32(time.Now().UnixNano()&0x7fffffff))
		return res, err
	}
	res.ID, err = s.core.engine.Move(a, m, to)
	if err != nil {
		return MoveResult{}, fmt.Errorf("couldn't move the message: %w", err)
	}
	s.core.SyncSoon()
	return res, nil
}

// OpenExternal opens a link from an email in the user's default browser.
// Only http, https and mailto links are allowed.
func (s *MailService) OpenExternal(link string) error {
	u, err := url.Parse(link)
	if err != nil {
		return err
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto":
		return application.Get().Browser.OpenURL(u.String())
	default:
		return errors.New("blocked link scheme: " + u.Scheme)
	}
}

func (s *MailService) load(id string) (store.MessageRow, store.Account, error) {
	m, err := s.core.store.GetMessage(id)
	if err != nil {
		return m, store.Account{}, err
	}
	a, err := s.core.store.GetAccount(m.AccountID)
	return m, a, err
}

var reURL = regexp.MustCompile(`https?://[^\s<>"']+`)

// textToHTML renders a plain-text body with paragraphs and clickable links.
func textToHTML(text string) string {
	esc := html.EscapeString(text)
	esc = reURL.ReplaceAllStringFunc(esc, func(u string) string {
		trimmed := strings.TrimRight(u, ".,;:)")
		return `<a href="` + trimmed + `">` + trimmed + `</a>` + u[len(trimmed):]
	})
	return `<div style="white-space: pre-wrap">` + esc + `</div>`
}
