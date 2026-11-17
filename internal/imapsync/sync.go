package imapsync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/HamzaBaghdadi/pigeon/internal/mail"
	"github.com/HamzaBaghdadi/pigeon/internal/mailparse"
	"github.com/HamzaBaghdadi/pigeon/internal/store"
)

// How much mail to keep locally per folder (newest first). Older mail stays on
// the server; deeper history comes later with paging and server search.
var windowByRole = map[mail.FolderRole]uint32{
	mail.RoleInbox:   500,
	mail.RoleSent:    200,
	mail.RoleDrafts:  100,
	mail.RoleArchive: 200,
	mail.RoleSpam:    100,
	mail.RoleTrash:   100,
}

const (
	bodyPrefetchMax  = 512 << 10 // download bodies up to 512 KB ahead of time
	bodyPrefetchEach = 50        // per folder per sync
)

// Credentials are how an account signs in: a password or an OAuth access token.
type Credentials struct {
	Password    string
	AccessToken string
}

// CredentialsFunc returns fresh credentials for an account (refreshing OAuth tokens as needed).
type CredentialsFunc func(a store.Account) (Credentials, error)

// Engine syncs accounts and performs mailbox actions. One cached connection
// per account is reused and serialized with a mutex.
type Engine struct {
	store *store.Store
	creds CredentialsFunc

	mu    sync.Mutex
	conns map[string]*session

	// OnNewMail is called with messages that arrived in an inbox since the
	// previous sync (never on an account's first sync).
	OnNewMail func(a store.Account, msgs []store.MessageRow)
}

type session struct {
	mu       sync.Mutex
	client   *imapclient.Client
	selected string
}

func NewEngine(st *store.Store, creds CredentialsFunc) *Engine {
	return &Engine{store: st, creds: creds, conns: map[string]*session{}}
}

// ConfigFor builds connection settings for a stored account.
func (e *Engine) ConfigFor(a store.Account) (Config, error) {
	cr, err := e.creds(a)
	if err != nil {
		return Config{}, err
	}
	user := a.Username
	if user == "" {
		user = a.Email
	}
	return Config{Host: a.IMAPHost, Port: a.IMAPPort, Security: Security(a.IMAPSecurity), Username: user, Password: cr.Password, AccessToken: cr.AccessToken}, nil
}

// with runs fn on the account's connection, reconnecting once if it dropped.
func (e *Engine) with(a store.Account, fn func(s *session) error) error {
	e.mu.Lock()
	s := e.conns[a.ID]
	if s == nil {
		s = &session{}
		e.conns[a.ID] = s
	}
	e.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	for attempt := 0; attempt < 2; attempt++ {
		if s.client == nil || isClosed(s.client) {
			cfg, err := e.ConfigFor(a)
			if err != nil {
				return err
			}
			c, err := Connect(cfg)
			if err != nil {
				return err
			}
			s.client, s.selected = c, ""
		}
		err := fn(s)
		if err == nil || !isConnErr(err, s.client) {
			return err
		}
		s.client.Close()
		s.client = nil
	}
	return errors.New("lost connection to the server")
}

// Disconnect closes an account's cached connection (e.g. when it's removed).
func (e *Engine) Disconnect(accountID string) {
	e.mu.Lock()
	s := e.conns[accountID]
	delete(e.conns, accountID)
	e.mu.Unlock()
	if s != nil {
		s.mu.Lock()
		if s.client != nil {
			s.client.Close()
		}
		s.mu.Unlock()
	}
}

func isClosed(c *imapclient.Client) bool {
	select {
	case <-c.Closed():
		return true
	default:
		return false
	}
}

func isConnErr(err error, c *imapclient.Client) bool {
	if c == nil || isClosed(c) {
		return true
	}
	var imapErr *imap.Error
	return !errors.As(err, &imapErr) // protocol errors (NO/BAD) are real errors; anything else is I/O
}

func (s *session) selectFolder(path string, readOnly bool) (*imap.SelectData, error) {
	data, err := s.client.Select(path, &imap.SelectOptions{ReadOnly: readOnly}).Wait()
	if err != nil {
		s.selected = ""
		return nil, err
	}
	s.selected = path
	if readOnly {
		s.selected = "ro:" + path
	}
	return data, nil
}

func (s *session) isSelected(path string) bool {
	return s.selected == path || s.selected == "ro:"+path
}

// ensureWritable selects path read-write unless it already is.
func (s *session) ensureWritable(path string) error {
	if s.selected == path {
		return nil
	}
	_, err := s.selectFolder(path, false)
	return err
}

// ---- full sync ----

// SyncAccount refreshes folders, recent messages, flags and bodies.
func (e *Engine) SyncAccount(ctx context.Context, a store.Account) error {
	var arrived []string
	defer func() { e.reportNew(a, arrived) }()
	return e.with(a, func(s *session) error {
		folders, err := e.syncFolders(s, a)
		if err != nil {
			return err
		}
		for _, f := range folders {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if _, ok := windowByRole[f.Role]; !ok {
				continue
			}
			ids, err := e.syncFolder(s, f)
			if err != nil {
				return fmt.Errorf("%s: %w", f.Name, err)
			}
			if f.Role == mail.RoleInbox {
				arrived = append(arrived, ids...)
			}
		}
		// Bodies second, so the message list fills in fast on first sync.
		for _, f := range folders {
			if _, ok := windowByRole[f.Role]; ok && ctx.Err() == nil {
				if err := e.prefetchBodies(s, f); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (e *Engine) syncFolders(s *session, a store.Account) ([]store.FolderRow, error) {
	list, err := s.client.List("", "*", &imap.ListOptions{ReturnSpecialUse: s.client.Caps().Has(imap.CapSpecialUse)}).Collect()
	if err != nil {
		return nil, err
	}
	taken := map[mail.FolderRole]bool{}
	var out []store.FolderRow
	var keep []string
	// Two passes: SPECIAL-USE attributes win over name guesses.
	for pass := 0; pass < 2; pass++ {
		for _, l := range list {
			if hasAttr(l.Attrs, imap.MailboxAttrNoSelect) || hasAttr(l.Attrs, imap.MailboxAttrNonExistent) {
				continue
			}
			id := store.FolderID(a.ID, l.Mailbox)
			if pass == 1 && contains(keep, id) {
				continue
			}
			role := roleFromAttrs(l.Attrs)
			if pass == 0 && role == mail.RoleCustom && !strings.EqualFold(l.Mailbox, "INBOX") {
				continue
			}
			if role == mail.RoleCustom {
				role = roleFromName(l.Mailbox, l.Delim)
			}
			if role != mail.RoleCustom && taken[role] {
				role = mail.RoleCustom
			}
			if role != mail.RoleCustom {
				taken[role] = true
			}
			f := store.FolderRow{Folder: mail.Folder{ID: id, AccountID: a.ID, Name: displayName(l.Mailbox, l.Delim), Role: role, Path: l.Mailbox}}
			if err := e.store.UpsertFolder(f); err != nil {
				return nil, err
			}
			keep = append(keep, id)
			out = append(out, f)
		}
	}
	if err := e.store.DeleteFoldersExcept(a.ID, keep); err != nil {
		return nil, err
	}
	return out, nil
}

// syncFolder refreshes one folder and returns the IDs of messages that
// arrived since the last sync (unread, with a higher UID than anything we
// had). A folder's first sync reports nothing, so linking an account doesn't
// produce a flood of "new mail".
func (e *Engine) syncFolder(s *session, f store.FolderRow) ([]string, error) {
	stored, err := e.store.GetFolder(f.ID)
	if err != nil {
		return nil, err
	}
	sel, err := s.selectFolder(f.Path, true)
	if err != nil {
		return nil, err
	}
	known := stored.UIDValidity != 0 && stored.UIDValidity == sel.UIDValidity
	if stored.UIDValidity != sel.UIDValidity {
		if err := e.store.ResetFolder(f.ID, sel.UIDValidity); err != nil {
			return nil, err
		}
	}
	local, err := e.store.FolderUIDs(f.ID)
	if err != nil {
		return nil, err
	}
	var maxLocal uint32
	for uid := range local {
		if uid > maxLocal {
			maxLocal = uid
		}
	}
	var arrived []string
	seen := map[uint32]bool{}
	var windowStart uint32 // lowest UID in the newest-N window
	if sel.NumMessages > 0 {
		start := uint32(1)
		if w := windowFor(f.Role); sel.NumMessages > w {
			start = sel.NumMessages - w + 1
		}
		var seq imap.SeqSet
		seq.AddRange(start, sel.NumMessages)
		msgs, err := s.client.Fetch(seq, &imap.FetchOptions{
			UID: true, Flags: true, Envelope: true, InternalDate: true, RFC822Size: true,
		}).Collect()
		if err != nil {
			return nil, err
		}
		for _, m := range msgs {
			uid := uint32(m.UID)
			seen[uid] = true
			if windowStart == 0 || uid < windowStart {
				windowStart = uid
			}
			row := rowFromFetch(f, m)
			if err := e.store.UpsertMessage(row); err != nil {
				return nil, err
			}
			if known && uid > maxLocal && row.Unread {
				arrived = append(arrived, row.ID)
			}
		}
	}

	switch {
	case sel.NumMessages == 0:
		err = e.store.SetFolderFloor(f.ID, 1) // empty folder: fully loaded
	case windowStart != 0 && (stored.FloorUID == 0 || !known || windowStart < stored.FloorUID):
		err = e.store.SetFolderFloor(f.ID, windowStart)
	}
	if err != nil {
		return nil, err
	}

	// Inside the window, anything the server no longer lists is gone.
	// Older mail the user loaded is checked separately so it isn't dropped.
	var older []imap.UID
	for uid, id := range local {
		switch {
		case seen[uid]:
		case windowStart != 0 && uid < windowStart:
			older = append(older, imap.UID(uid))
		default:
			if err := e.store.DeleteMessage(id); err != nil {
				return nil, err
			}
		}
	}
	if len(older) > 0 {
		if err := e.refreshOlder(s, f, local, older); err != nil {
			return nil, err
		}
	}
	return arrived, nil
}

// refreshOlder updates flags for previously loaded older messages and removes
// the ones deleted on the server.
func (e *Engine) refreshOlder(s *session, f store.FolderRow, local map[uint32]string, uids []imap.UID) error {
	msgs, err := s.client.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{UID: true, Flags: true}).Collect()
	if err != nil {
		return err
	}
	still := map[uint32]bool{}
	for _, m := range msgs {
		uid := uint32(m.UID)
		still[uid] = true
		unread, starred := !hasFlag(m.Flags, imap.FlagSeen), hasFlag(m.Flags, imap.FlagFlagged)
		if err := e.store.SetFlags(local[uid], &unread, &starred); err != nil {
			return err
		}
	}
	for _, uid := range uids {
		if !still[uint32(uid)] {
			if err := e.store.DeleteMessage(local[uint32(uid)]); err != nil {
				return err
			}
		}
	}
	return nil
}

// windowFor is how many recent messages are kept in sync for a folder.
func windowFor(role mail.FolderRole) uint32 {
	if w, ok := windowByRole[role]; ok {
		return w
	}
	return 100 // folders and labels the user opens
}

// SyncInbox quickly refreshes just the inbox (used when the server pushes a
// change). Falls back to a full sync if folders aren't known yet.
func (e *Engine) SyncInbox(ctx context.Context, a store.Account) error {
	inbox, err := e.store.FolderByRole(a.ID, mail.RoleInbox)
	if err != nil {
		return e.SyncAccount(ctx, a)
	}
	var arrived []string
	err = e.with(a, func(s *session) error {
		var err error
		if arrived, err = e.syncFolder(s, inbox); err != nil {
			return err
		}
		return e.prefetchBodies(s, inbox)
	})
	e.reportNew(a, arrived)
	return err
}

// reportNew passes newly arrived messages (with bodies, for previews) to OnNewMail.
func (e *Engine) reportNew(a store.Account, ids []string) {
	if e.OnNewMail == nil || len(ids) == 0 {
		return
	}
	var rows []store.MessageRow
	for _, id := range ids {
		if m, err := e.store.GetMessage(id); err == nil {
			rows = append(rows, m)
		}
	}
	if len(rows) > 0 {
		e.OnNewMail(a, rows)
	}
}

func rowFromFetch(f store.FolderRow, m *imapclient.FetchMessageBuffer) store.MessageRow {
	r := store.MessageRow{
		MessageSummary: mail.MessageSummary{
			ID:        store.MessageID(f.ID, uint32(m.UID)),
			AccountID: f.AccountID,
			FolderID:  f.ID,
			Date:      m.InternalDate,
			Unread:    !hasFlag(m.Flags, imap.FlagSeen),
			Starred:   hasFlag(m.Flags, imap.FlagFlagged),
			To:        []mail.Address{},
		},
		UID:  uint32(m.UID),
		Size: m.RFC822Size,
	}
	if env := m.Envelope; env != nil {
		r.Subject = env.Subject
		r.MessageID = env.MessageID
		if !env.Date.IsZero() {
			r.Date = env.Date
		}
		if len(env.From) > 0 {
			r.From = mail.Address{Name: env.From[0].Name, Email: env.From[0].Addr()}
		}
		for _, t := range env.To {
			r.To = append(r.To, mail.Address{Name: t.Name, Email: t.Addr()})
		}
		for _, c := range env.Cc {
			r.Cc = append(r.Cc, mail.Address{Name: c.Name, Email: c.Addr()})
		}
	}
	return r
}

func (e *Engine) prefetchBodies(s *session, f store.FolderRow) error {
	missing, err := e.store.MissingBodies(f.ID, bodyPrefetchMax, bodyPrefetchEach)
	if err != nil || len(missing) == 0 {
		return err
	}
	if !s.isSelected(f.Path) {
		if _, err := s.selectFolder(f.Path, true); err != nil {
			return err
		}
	}
	uids := make([]imap.UID, len(missing))
	byUID := map[uint32]string{}
	for i, m := range missing {
		uids[i] = imap.UID(m.UID)
		byUID[m.UID] = m.ID
	}
	return e.fetchBodies(s, imap.UIDSetNum(uids...), byUID)
}

var fullBody = &imap.FetchItemBodySection{Peek: true} // BODY.PEEK[]: doesn't mark as read

func (e *Engine) fetchBodies(s *session, uids imap.UIDSet, byUID map[uint32]string) error {
	msgs, err := s.client.Fetch(uids, &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{fullBody}}).Collect()
	if err != nil {
		return err
	}
	for _, m := range msgs {
		id, ok := byUID[uint32(m.UID)]
		if !ok {
			continue
		}
		body, err := mailparse.Parse(m.FindBodySection(fullBody))
		if err != nil {
			body = mailparse.Body{Text: "(Pigeon couldn't read this message's content.)"}
		}
		parts := make([]store.Part, len(body.Parts))
		for i, p := range body.Parts {
			parts[i] = store.Part{Attachment: mail.Attachment{
				Index: p.Index, Filename: p.Filename, ContentType: p.ContentType,
				Size: p.Size, ContentID: p.ContentID, Inline: p.Inline,
			}, Data: p.Data}
		}
		if err := e.store.SetBody(id, body.HTML, body.Text, body.Snippet, body.HasAttachments, parts); err != nil {
			return err
		}
	}
	return nil
}

// ---- actions ----

// FetchBody downloads one message's body if it isn't stored yet.
func (e *Engine) FetchBody(a store.Account, m store.MessageRow) error {
	f, err := e.store.GetFolder(m.FolderID)
	if err != nil {
		return err
	}
	return e.with(a, func(s *session) error {
		if !s.isSelected(f.Path) {
			if _, err := s.selectFolder(f.Path, true); err != nil {
				return err
			}
		}
		return e.fetchBodies(s, imap.UIDSetNum(imap.UID(m.UID)), map[uint32]string{m.UID: m.ID})
	})
}

// SetFlag adds or removes \Seen or \Flagged on the server.
func (e *Engine) SetFlag(a store.Account, m store.MessageRow, flag imap.Flag, on bool) error {
	f, err := e.store.GetFolder(m.FolderID)
	if err != nil {
		return err
	}
	op := imap.StoreFlagsDel
	if on {
		op = imap.StoreFlagsAdd
	}
	return e.with(a, func(s *session) error {
		if err := s.ensureWritable(f.Path); err != nil {
			return err
		}
		return s.client.Store(imap.UIDSetNum(imap.UID(m.UID)), &imap.StoreFlags{Op: op, Silent: true, Flags: []imap.Flag{flag}}, nil).Close()
	})
}

// Move moves a message to another folder and returns its new local ID
// ("" if the server didn't report the new UID; the next sync will pick it up).
func (e *Engine) Move(a store.Account, m store.MessageRow, to store.FolderRow) (string, error) {
	from, err := e.store.GetFolder(m.FolderID)
	if err != nil {
		return "", err
	}
	var newID string
	err = e.with(a, func(s *session) error {
		if err := s.ensureWritable(from.Path); err != nil {
			return err
		}
		data, err := s.client.Move(imap.UIDSetNum(imap.UID(m.UID)), to.Path).Wait()
		if err != nil {
			return err
		}
		if data != nil {
			if dest, ok := data.DestUIDs.(imap.UIDSet); ok {
				if nums, ok := dest.Nums(); ok && len(nums) == 1 {
					newID, err = e.store.MoveLocal(m.ID, to.ID, uint32(nums[0]))
					return err
				}
			}
		}
		return e.store.DeleteMessage(m.ID)
	})
	return newID, err
}

// ---- helpers ----

func hasAttr(attrs []imap.MailboxAttr, a imap.MailboxAttr) bool {
	for _, x := range attrs {
		if strings.EqualFold(string(x), string(a)) {
			return true
		}
	}
	return false
}

func hasFlag(flags []imap.Flag, f imap.Flag) bool {
	for _, x := range flags {
		if strings.EqualFold(string(x), string(f)) {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func roleFromAttrs(attrs []imap.MailboxAttr) mail.FolderRole {
	switch {
	case hasAttr(attrs, imap.MailboxAttrSent):
		return mail.RoleSent
	case hasAttr(attrs, imap.MailboxAttrDrafts):
		return mail.RoleDrafts
	case hasAttr(attrs, imap.MailboxAttrTrash):
		return mail.RoleTrash
	case hasAttr(attrs, imap.MailboxAttrJunk):
		return mail.RoleSpam
	case hasAttr(attrs, imap.MailboxAttrArchive):
		return mail.RoleArchive
	}
	return mail.RoleCustom
}

// roleFromName guesses roles for servers without SPECIAL-USE.
func roleFromName(path string, delim rune) mail.FolderRole {
	if strings.EqualFold(path, "INBOX") {
		return mail.RoleInbox
	}
	name := strings.ToLower(displayName(path, delim))
	switch name {
	case "sent", "sent items", "sent mail", "sent messages":
		return mail.RoleSent
	case "drafts", "draft":
		return mail.RoleDrafts
	case "trash", "deleted items", "deleted messages", "bin":
		return mail.RoleTrash
	case "spam", "junk", "junk e-mail", "junk email", "bulk mail":
		return mail.RoleSpam
	case "archive", "archives", "all mail":
		return mail.RoleArchive
	}
	return mail.RoleCustom
}

func displayName(path string, delim rune) string {
	if strings.EqualFold(path, "INBOX") {
		return "Inbox"
	}
	if delim != 0 {
		if i := strings.LastIndex(path, string(delim)); i >= 0 {
			return path[i+1:]
		}
	}
	return path
}

// AppendSent stores a copy of a sent message in the account's Sent folder
// (marked read). Providers whose SMTP servers file sent mail themselves
// (Gmail, Outlook) don't need this.
func (e *Engine) AppendSent(a store.Account, raw []byte) error {
	sent, err := e.store.FolderByRole(a.ID, mail.RoleSent)
	if err != nil {
		return fmt.Errorf("no Sent folder: %w", err)
	}
	return e.with(a, func(s *session) error {
		cmd := s.client.Append(sent.Path, int64(len(raw)), &imap.AppendOptions{
			Flags: []imap.Flag{imap.FlagSeen},
			Time:  time.Now(),
		})
		if _, err := cmd.Write(raw); err != nil {
			cmd.Close()
			return err
		}
		if err := cmd.Close(); err != nil {
			return err
		}
		_, err := cmd.Wait()
		return err
	})
}
