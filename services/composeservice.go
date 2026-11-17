package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/HamzaBaghdadi/pigeon/internal/compose"
	"github.com/HamzaBaghdadi/pigeon/internal/mail"
	"github.com/HamzaBaghdadi/pigeon/internal/mailparse"
	"github.com/HamzaBaghdadi/pigeon/internal/smtpsend"
	"github.com/HamzaBaghdadi/pigeon/internal/store"
)

// ComposeService writes and sends mail.
type ComposeService struct {
	core *Core

	mu          sync.Mutex
	attachments map[string]attachmentSource // ticket -> what to attach
}

func NewComposeService(core *Core) *ComposeService {
	return &ComposeService{core: core, attachments: map[string]attachmentSource{}}
}

// attachmentSource is a file the user picked, or an attachment of a message
// being forwarded.
type attachmentSource struct {
	path      string
	messageID string
	index     int
}

func (s *ComposeService) loadMessage(id string) (store.MessageRow, store.Account, error) {
	m, err := s.core.store.GetMessage(id)
	if err != nil {
		return m, store.Account{}, err
	}
	a, err := s.core.store.GetAccount(m.AccountID)
	return m, a, err
}

// Draft is what the composer edits and sends.
type Draft struct {
	AccountID string         `json:"accountId"`
	To        []mail.Address `json:"to"`
	Cc        []mail.Address `json:"cc"`
	Bcc       []mail.Address `json:"bcc"`
	Subject   string         `json:"subject"`
	Body      string         `json:"body"`
	// ReplyToID is the local ID of the message being answered (threading).
	ReplyToID string `json:"replyToId"`
	// Attachments are tickets from PickAttachments.
	Attachments []AttachmentInfo `json:"attachments"`
}

// AttachmentInfo describes a file picked for sending. ID is an opaque ticket:
// the UI never handles file paths, so it can only attach files the user chose
// in the native dialog.
type AttachmentInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// ComposeMode is how a draft starts.
type ComposeMode string

const (
	ModeReply    ComposeMode = "reply"
	ModeReplyAll ComposeMode = "replyAll"
	ModeForward  ComposeMode = "forward"
)

// NewDraft returns a draft for replying to, replying all to, or forwarding a message.
func (s *ComposeService) NewDraft(messageID string, mode ComposeMode) (Draft, error) {
	m, err := s.core.store.GetMessage(messageID)
	if err != nil {
		return Draft{}, err
	}
	a, err := s.core.store.GetAccount(m.AccountID)
	if err != nil {
		return Draft{}, err
	}
	if !m.BodyFetched && !a.Demo {
		if err := s.core.engine.FetchBody(a, m); err == nil {
			m, _ = s.core.store.GetMessage(messageID)
		}
	}
	text := m.BodyText
	if strings.TrimSpace(text) == "" && m.BodyHTML != "" {
		text = mailparse.HTMLToText(m.BodyHTML)
	}

	d := Draft{AccountID: a.ID, To: []mail.Address{}, Cc: []mail.Address{}, Bcc: []mail.Address{}, Attachments: []AttachmentInfo{}}
	self := strings.ToLower(a.Email)
	switch mode {
	case ModeForward:
		d.Subject = compose.ReplySubject("Fwd", m.Subject)
		d.Body = forwardBlock(m.MessageSummary, text)
		if !a.Demo {
			if parts, err := s.core.store.ListParts(m.ID); err == nil {
				s.mu.Lock()
				for _, p := range parts {
					if p.Inline {
						continue
					}
					ticket := newTicket()
					s.attachments[ticket] = attachmentSource{messageID: m.ID, index: p.Index}
					d.Attachments = append(d.Attachments, AttachmentInfo{ID: ticket, Name: safeFilename(p.Filename, p.Index), Size: p.Size})
				}
				s.mu.Unlock()
			}
		}
	case ModeReply, ModeReplyAll:
		d.ReplyToID = m.ID
		d.Subject = compose.ReplySubject("Re", m.Subject)
		d.Body = compose.QuoteForReply(m.From, m.Date, text)
		// Replying to your own sent message goes to its recipients.
		if strings.ToLower(m.From.Email) == self {
			d.To = append(d.To, m.To...)
		} else {
			d.To = append(d.To, m.From)
		}
		if mode == ModeReplyAll {
			seen := map[string]bool{self: true}
			for _, x := range d.To {
				seen[strings.ToLower(x.Email)] = true
			}
			for _, x := range append(append([]mail.Address{}, m.To...), m.Cc...) {
				if k := strings.ToLower(x.Email); x.Email != "" && !seen[k] {
					seen[k] = true
					d.Cc = append(d.Cc, x)
				}
			}
		}
	default:
		return Draft{}, fmt.Errorf("unknown compose mode %q", mode)
	}
	return d, nil
}

func forwardBlock(m mail.MessageSummary, text string) string {
	var b strings.Builder
	b.WriteString("\n\n---------- Forwarded message ----------\n")
	fmt.Fprintf(&b, "From: %s\n", formatAddr(m.From))
	fmt.Fprintf(&b, "Date: %s\n", m.Date.Format("Mon, Jan 2, 2006 at 3:04 PM"))
	fmt.Fprintf(&b, "Subject: %s\n", m.Subject)
	if len(m.To) > 0 {
		parts := make([]string, len(m.To))
		for i, t := range m.To {
			parts[i] = formatAddr(t)
		}
		fmt.Fprintf(&b, "To: %s\n", strings.Join(parts, ", "))
	}
	b.WriteString("\n")
	b.WriteString(compose.Tidy(text))
	b.WriteString("\n")
	return b.String()
}

func formatAddr(a mail.Address) string {
	if a.Name == "" {
		return a.Email
	}
	return a.Name + " <" + a.Email + ">"
}

// SuggestContacts returns known addresses matching what's typed.
func (s *ComposeService) SuggestContacts(prefix string) ([]mail.Address, error) {
	return s.core.store.SuggestContacts(prefix, 8)
}

// PickAttachments opens the native file picker.
func (s *ComposeService) PickAttachments() ([]AttachmentInfo, error) {
	app := application.Get()
	if app == nil {
		return nil, errors.New("app not running")
	}
	paths, err := app.Dialog.OpenFile().CanChooseFiles(true).SetTitle("Attach files").PromptForMultipleSelection()
	if err != nil {
		return nil, err
	}
	out := []AttachmentInfo{}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			continue
		}
		ticket := newTicket()
		s.attachments[ticket] = attachmentSource{path: p}
		out = append(out, AttachmentInfo{ID: ticket, Name: filepath.Base(p), Size: st.Size()})
	}
	return out, nil
}

// Send builds and sends a draft, then files a copy in Sent.
func (s *ComposeService) Send(d Draft) error {
	a, err := s.core.store.GetAccount(d.AccountID)
	if err != nil {
		return errors.New("choose which account to send from")
	}
	msg := compose.Message{
		From:    mail.Address{Name: s.core.prefs.Get().DisplayName, Email: a.Email},
		To:      clean(d.To),
		Cc:      clean(d.Cc),
		Bcc:     clean(d.Bcc),
		Subject: d.Subject,
		Text:    d.Body,
		Date:    time.Now(),
	}
	if d.ReplyToID != "" {
		if orig, err := s.core.store.GetMessage(d.ReplyToID); err == nil {
			msg.InReplyTo = orig.MessageID
		}
	}
	raws := map[string][]byte{} // forwarded messages, downloaded once
	for _, att := range d.Attachments {
		s.mu.Lock()
		src, ok := s.attachments[att.ID]
		s.mu.Unlock()
		if !ok {
			return fmt.Errorf("attachment %q is no longer available; attach it again", att.Name)
		}
		if src.messageID == "" {
			data, err := os.ReadFile(src.path)
			if err != nil {
				return fmt.Errorf("couldn't read %s: %w", att.Name, err)
			}
			msg.Attachments = append(msg.Attachments, compose.Attachment{Name: filepath.Base(src.path), Data: data})
			continue
		}
		// An attachment of the message being forwarded.
		raw, ok := raws[src.messageID]
		if !ok {
			orig, oa, err := s.loadMessage(src.messageID)
			if err != nil {
				return fmt.Errorf("couldn't get %s from the original message: %w", att.Name, err)
			}
			if raw, err = s.core.engine.FetchRaw(oa, orig); err != nil {
				return fmt.Errorf("couldn't download %s: %w", att.Name, err)
			}
			raws[src.messageID] = raw
		}
		part, data, err := mailparse.PartData(raw, src.index)
		if err != nil {
			return fmt.Errorf("couldn't find %s in the original message", att.Name)
		}
		msg.Attachments = append(msg.Attachments, compose.Attachment{Name: safeFilename(part.Filename, src.index), ContentType: part.ContentType, Data: data})
	}
	raw, _, err := compose.Build(msg)
	if err != nil {
		return err
	}

	if a.Demo {
		return s.saveDemoSent(a, msg, raw)
	}

	creds, err := s.core.credentials(a)
	if err != nil {
		return err
	}
	user := a.Username
	if user == "" {
		user = a.Email
	}
	if err := smtpsend.Send(smtpsend.Config{
		Host: a.SMTPHost, Port: a.SMTPPort, Security: a.SMTPSecurity,
		Username: user, Password: creds.Password, AccessToken: creds.AccessToken,
	}, a.Email, msg.Recipients(), raw); err != nil {
		return err
	}
	for _, att := range d.Attachments {
		s.mu.Lock()
		delete(s.attachments, att.ID)
		s.mu.Unlock()
	}
	if !serverFilesSentMail(a) {
		// Sent successfully; a failed copy shouldn't make the UI report failure.
		_ = s.core.engine.AppendSent(a, raw)
	}
	s.core.SyncSoon()
	return nil
}

// serverFilesSentMail reports providers whose SMTP servers save a copy in
// Sent automatically; appending another would create a duplicate.
func serverFilesSentMail(a store.Account) bool {
	h := strings.ToLower(a.SMTPHost)
	return a.Provider == mail.ProviderGmail || strings.HasSuffix(h, "gmail.com") ||
		strings.HasSuffix(h, "office365.com") || strings.HasSuffix(h, "outlook.com")
}

func (s *ComposeService) saveDemoSent(a store.Account, msg compose.Message, raw []byte) error {
	sent, err := s.core.store.FolderByRole(a.ID, mail.RoleSent)
	if err != nil {
		return err
	}
	body, _ := mailparse.Parse(raw)
	uid := uint32(time.Now().UnixNano() & 0x7fffffff)
	err = s.core.store.UpsertMessage(store.MessageRow{
		MessageSummary: mail.MessageSummary{
			ID: store.MessageID(sent.ID, uid), AccountID: a.ID, FolderID: sent.ID,
			From: msg.From, To: msg.To, Cc: msg.Cc, Subject: msg.Subject, Snippet: body.Snippet,
			Date: msg.Date, HasAttachments: len(msg.Attachments) > 0,
		},
		UID: uid, BodyHTML: body.HTML, BodyText: body.Text, BodyFetched: true,
	})
	if err == nil {
		emit(EventMailChanged, a.ID)
	}
	return err
}

func clean(list []mail.Address) []mail.Address {
	out := []mail.Address{}
	for _, a := range list {
		a.Email = strings.TrimSpace(a.Email)
		a.Name = strings.TrimSpace(a.Name)
		if a.Email != "" {
			out = append(out, a)
		}
	}
	return out
}

func newTicket() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
