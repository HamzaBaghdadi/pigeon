package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/HamzaBaghdadi/pigeon/internal/mail"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

// Account is a stored account including its server settings.
type Account struct {
	mail.Account
	Username     string
	IMAPHost     string
	IMAPPort     int
	IMAPSecurity string // "tls" or "starttls"
	SMTPHost     string
	SMTPPort     int
	SMTPSecurity string
}

// How an account signs in (mail.Account.Auth).
const (
	AuthPassword  = "password"
	AuthGoogle    = "google"
	AuthMicrosoft = "microsoft"
)

// ---- accounts ----

func (s *Store) SaveAccount(a Account) error {
	_, err := s.db.Exec(`INSERT INTO accounts
		(id, name, email, provider, color, username, imap_host, imap_port, imap_security, smtp_host, smtp_port, smtp_security, demo, auth)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, email=excluded.email, provider=excluded.provider,
			color=excluded.color, username=excluded.username, imap_host=excluded.imap_host, imap_port=excluded.imap_port,
			imap_security=excluded.imap_security, smtp_host=excluded.smtp_host, smtp_port=excluded.smtp_port,
			smtp_security=excluded.smtp_security, demo=excluded.demo, auth=excluded.auth`,
		a.ID, a.Name, a.Email, string(a.Provider), a.Color, a.Username, a.IMAPHost, a.IMAPPort, a.IMAPSecurity,
		a.SMTPHost, a.SMTPPort, a.SMTPSecurity, a.Demo, authOrDefault(a.Auth))
	return err
}

const accountCols = `id, name, email, provider, color, username, imap_host, imap_port, imap_security, smtp_host, smtp_port, smtp_security, demo, auth`

func authOrDefault(a string) string {
	if a == "" {
		return AuthPassword
	}
	return a
}

func scanAccount(sc interface{ Scan(...any) error }) (Account, error) {
	var a Account
	var provider string
	err := sc.Scan(&a.ID, &a.Name, &a.Email, &provider, &a.Color, &a.Username, &a.IMAPHost, &a.IMAPPort,
		&a.IMAPSecurity, &a.SMTPHost, &a.SMTPPort, &a.SMTPSecurity, &a.Demo, &a.Auth)
	a.Provider = mail.Provider(provider)
	return a, err
}

func (s *Store) ListAccounts() ([]Account, error) {
	rows, err := s.db.Query(`SELECT ` + accountCols + ` FROM accounts ORDER BY demo, created_at, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetAccount(id string) (Account, error) {
	a, err := scanAccount(s.db.QueryRow(`SELECT `+accountCols+` FROM accounts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// DeleteAccount removes an account and (via cascade) its folders and messages.
func (s *Store) DeleteAccount(id string) error {
	_, err := s.db.Exec(`DELETE FROM accounts WHERE id = ?`, id)
	return err
}

// DeleteDemoAccounts removes all sample data.
func (s *Store) DeleteDemoAccounts() error {
	_, err := s.db.Exec(`DELETE FROM accounts WHERE demo = 1`)
	return err
}

// ---- folders ----

// FolderRow is a stored folder with its server path and sync state.
type FolderRow struct {
	mail.Folder
	UIDValidity uint32
	// FloorUID is the lowest UID down to which the folder's history is
	// loaded without gaps (0 = not synced yet, 1 = everything).
	FloorUID uint32
}

func FolderID(accountID, path string) string { return accountID + "|" + path }

func (s *Store) UpsertFolder(f FolderRow) error {
	_, err := s.db.Exec(`INSERT INTO folders (id, account_id, path, name, role, uidvalidity) VALUES (?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, role=excluded.role`,
		f.ID, f.AccountID, f.Path, f.Name, string(f.Role), f.UIDValidity)
	return err
}

// ResetFolder clears a folder's messages and stores a new UIDVALIDITY
// (the server invalidated every UID we had).
func (s *Store) ResetFolder(folderID string, uidValidity uint32) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM messages WHERE folder_id = ?`, folderID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE folders SET uidvalidity = ?, floor_uid = 0 WHERE id = ?`, uidValidity, folderID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteFoldersExcept removes folders of an account that no longer exist on the server.
func (s *Store) DeleteFoldersExcept(accountID string, keep []string) error {
	if len(keep) == 0 {
		_, err := s.db.Exec(`DELETE FROM folders WHERE account_id = ?`, accountID)
		return err
	}
	args := []any{accountID}
	ph := make([]string, len(keep))
	for i, id := range keep {
		ph[i] = "?"
		args = append(args, id)
	}
	_, err := s.db.Exec(`DELETE FROM folders WHERE account_id = ? AND id NOT IN (`+strings.Join(ph, ",")+`)`, args...)
	return err
}

const folderCols = `f.id, f.account_id, f.path, f.name, f.role, f.uidvalidity, f.floor_uid,
	(SELECT count(*) FROM messages m WHERE m.folder_id = f.id AND m.unread = 1)`

func scanFolder(sc interface{ Scan(...any) error }) (FolderRow, error) {
	var f FolderRow
	var role string
	err := sc.Scan(&f.ID, &f.AccountID, &f.Path, &f.Name, &role, &f.UIDValidity, &f.FloorUID, &f.Unread)
	f.Role = mail.FolderRole(role)
	return f, err
}

// ListFolders returns folders for one account, or all accounts if accountID is empty.
func (s *Store) ListFolders(accountID string) ([]FolderRow, error) {
	rows, err := s.db.Query(`SELECT `+folderCols+` FROM folders f WHERE ? = '' OR f.account_id = ? ORDER BY f.account_id, f.name`,
		accountID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FolderRow
	for rows.Next() {
		f, err := scanFolder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) GetFolder(id string) (FolderRow, error) {
	f, err := scanFolder(s.db.QueryRow(`SELECT `+folderCols+` FROM folders f WHERE f.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return f, ErrNotFound
	}
	return f, err
}

// FolderByRole finds an account's folder with a well-known role.
func (s *Store) FolderByRole(accountID string, role mail.FolderRole) (FolderRow, error) {
	f, err := scanFolder(s.db.QueryRow(`SELECT `+folderCols+` FROM folders f WHERE f.account_id = ? AND f.role = ? LIMIT 1`,
		accountID, string(role)))
	if errors.Is(err, sql.ErrNoRows) {
		return f, ErrNotFound
	}
	return f, err
}

// ---- messages ----

// MessageRow is a stored message.
type MessageRow struct {
	mail.MessageSummary
	UID          uint32
	MessageID    string
	Size         int64
	BodyHTML     string
	BodyText     string
	BodyFetched  bool
	PartsIndexed bool
}

func MessageID(folderID string, uid uint32) string { return fmt.Sprintf("%s|%d", folderID, uid) }

// UpsertMessage inserts a message or refreshes its envelope and flags,
// keeping any body already downloaded.
func (s *Store) UpsertMessage(m MessageRow) error {
	to, _ := json.Marshal(nonNil(m.To))
	cc, _ := json.Marshal(nonNil(m.Cc))
	_, err := s.db.Exec(`INSERT INTO messages
		(id, account_id, folder_id, uid, message_id, from_name, from_email, to_json, cc_json, subject, snippet, date, size,
		 unread, starred, has_attachments, body_html, body_text, body_fetched)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET unread=excluded.unread, starred=excluded.starred,
			subject=excluded.subject, from_name=excluded.from_name, from_email=excluded.from_email, cc_json=excluded.cc_json`,
		m.ID, m.AccountID, m.FolderID, m.UID, m.MessageID, m.From.Name, m.From.Email, string(to), string(cc), m.Subject, m.Snippet,
		m.Date.Unix(), m.Size, m.Unread, m.Starred, m.HasAttachments, m.BodyHTML, m.BodyText, m.BodyFetched)
	return err
}

// SetBody stores a downloaded, parsed body.
func (s *Store) SetBody(id, html, text, snippet string, hasAttachments bool, parts []Part) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE messages SET body_html=?, body_text=?, snippet=?, has_attachments=?, body_fetched=1, parts_indexed=1 WHERE id=?`,
		html, text, snippet, hasAttachments, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM parts WHERE message_id = ?`, id); err != nil {
		return err
	}
	for _, p := range parts {
		if _, err := tx.Exec(`INSERT INTO parts (message_id, idx, filename, content_type, size, content_id, inline, data) VALUES (?,?,?,?,?,?,?,?)`,
			id, p.Index, p.Filename, p.ContentType, p.Size, p.ContentID, p.Inline, p.Data); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Part is a stored attachment or inline image (Data only for small inline images).
type Part struct {
	mail.Attachment
	Data []byte
}

// ListParts returns a message's attachments and inline images (without data).
func (s *Store) ListParts(messageID string) ([]mail.Attachment, error) {
	rows, err := s.db.Query(`SELECT idx, filename, content_type, size, content_id, inline FROM parts WHERE message_id = ? ORDER BY idx`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []mail.Attachment{}
	for rows.Next() {
		var a mail.Attachment
		if err := rows.Scan(&a.Index, &a.Filename, &a.ContentType, &a.Size, &a.ContentID, &a.Inline); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// InlineImages returns content-id -> (content type, data) for a message's stored inline images.
func (s *Store) InlineImages(messageID string) (map[string]Part, error) {
	rows, err := s.db.Query(`SELECT idx, content_type, content_id, data FROM parts WHERE message_id = ? AND inline = 1 AND data IS NOT NULL`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Part{}
	for rows.Next() {
		var p Part
		if err := rows.Scan(&p.Index, &p.ContentType, &p.ContentID, &p.Data); err != nil {
			return nil, err
		}
		out[p.ContentID] = p
	}
	return out, rows.Err()
}

// SetFlags updates the unread and/or starred flags. Nil leaves a flag unchanged.
func (s *Store) SetFlags(id string, unread, starred *bool) error {
	if unread != nil {
		if _, err := s.db.Exec(`UPDATE messages SET unread=? WHERE id=?`, *unread, id); err != nil {
			return err
		}
	}
	if starred != nil {
		if _, err := s.db.Exec(`UPDATE messages SET starred=? WHERE id=?`, *starred, id); err != nil {
			return err
		}
	}
	return nil
}

// MoveLocal re-files a message under a new folder and UID and returns its new ID.
func (s *Store) MoveLocal(id, folderID string, uid uint32) (string, error) {
	newID := MessageID(folderID, uid)
	_, err := s.db.Exec(`UPDATE messages SET id=?, folder_id=?, uid=? WHERE id=?`, newID, folderID, uid, id)
	return newID, err
}

func (s *Store) DeleteMessage(id string) error {
	_, err := s.db.Exec(`DELETE FROM messages WHERE id = ?`, id)
	return err
}

// FolderUIDs returns uid -> message id for every stored message in a folder.
func (s *Store) FolderUIDs(folderID string) (map[uint32]string, error) {
	rows, err := s.db.Query(`SELECT uid, id FROM messages WHERE folder_id = ?`, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uint32]string{}
	for rows.Next() {
		var uid uint32
		var id string
		if err := rows.Scan(&uid, &id); err != nil {
			return nil, err
		}
		out[uid] = id
	}
	return out, rows.Err()
}

// MissingBodies lists messages in a folder whose body hasn't been downloaded,
// newest first, up to limit, skipping messages larger than maxSize.
func (s *Store) MissingBodies(folderID string, maxSize int64, limit int) ([]MessageRow, error) {
	rows, err := s.db.Query(`SELECT `+msgCols+` FROM messages WHERE folder_id = ? AND body_fetched = 0 AND size <= ?
		ORDER BY date DESC LIMIT ?`, folderID, maxSize, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

const msgCols = `id, account_id, folder_id, uid, message_id, from_name, from_email, to_json, cc_json, subject, snippet, date, size,
	unread, starred, has_attachments, body_html, body_text, body_fetched, parts_indexed`

func scanMessage(sc interface{ Scan(...any) error }) (MessageRow, error) {
	var m MessageRow
	var to, cc string
	var date int64
	err := sc.Scan(&m.ID, &m.AccountID, &m.FolderID, &m.UID, &m.MessageID, &m.From.Name, &m.From.Email, &to, &cc,
		&m.Subject, &m.Snippet, &date, &m.Size, &m.Unread, &m.Starred, &m.HasAttachments, &m.BodyHTML, &m.BodyText,
		&m.BodyFetched, &m.PartsIndexed)
	if err != nil {
		return m, err
	}
	m.Date = time.Unix(date, 0)
	_ = json.Unmarshal([]byte(to), &m.To)
	_ = json.Unmarshal([]byte(cc), &m.Cc)
	return m, nil
}

func scanMessages(rows *sql.Rows) ([]MessageRow, error) {
	var out []MessageRow
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) GetMessage(id string) (MessageRow, error) {
	m, err := scanMessage(s.db.QueryRow(`SELECT `+msgCols+` FROM messages WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

// ListMessages returns summaries for a folder role across one account (or all
// accounts when accountID is empty), newest first. A non-empty query runs a
// full-text search over subject, sender, snippet and body.
func (s *Store) ListMessages(accountID string, role mail.FolderRole, query string, limit int) ([]mail.MessageSummary, error) {
	where := []string{"(? = '' OR m.account_id = ?)"}
	args := []any{accountID, accountID}
	if role == mail.RoleStarred {
		where = append(where, "m.starred = 1")
	} else {
		where = append(where, "f.role = ?")
		args = append(args, string(role))
	}
	from := "messages m JOIN folders f ON f.id = m.folder_id"
	if fts := ftsQuery(query); fts != "" {
		from += " JOIN messages_fts ON messages_fts.rowid = m.rowid"
		where = append(where, "messages_fts MATCH ?")
		args = append(args, fts)
	}
	args = append(args, limit)
	rows, err := s.db.Query(`SELECT m.id, m.account_id, m.folder_id, m.from_name, m.from_email, m.to_json, m.subject,
		m.snippet, m.date, m.unread, m.starred, m.has_attachments
		FROM `+from+` WHERE `+strings.Join(where, " AND ")+` ORDER BY m.date DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []mail.MessageSummary{}
	for rows.Next() {
		var m mail.MessageSummary
		var to string
		var date int64
		if err := rows.Scan(&m.ID, &m.AccountID, &m.FolderID, &m.From.Name, &m.From.Email, &to, &m.Subject,
			&m.Snippet, &date, &m.Unread, &m.Starred, &m.HasAttachments); err != nil {
			return nil, err
		}
		m.Date = time.Unix(date, 0)
		_ = json.Unmarshal([]byte(to), &m.To)
		out = append(out, m)
	}
	return out, rows.Err()
}

// ftsQuery turns free text into a safe FTS5 query: every word must match,
// as a prefix, so "lin road" finds "Lina … roadmap".
func ftsQuery(q string) string {
	var terms []string
	for _, w := range strings.Fields(q) {
		w = strings.ReplaceAll(w, `"`, `""`)
		terms = append(terms, `"`+w+`"*`)
	}
	return strings.Join(terms, " ")
}

func nonNil(a []mail.Address) []mail.Address {
	if a == nil {
		return []mail.Address{}
	}
	return a
}

// SuggestContacts returns people whose name or address starts with prefix
// (or a word in their name does), most frequent correspondents first.
func (s *Store) SuggestContacts(prefix string, limit int) ([]mail.Address, error) {
	p := strings.ToLower(strings.TrimSpace(prefix))
	if p == "" {
		return []mail.Address{}, nil
	}
	esc := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(p)
	rows, err := s.db.Query(`SELECT from_name, from_email FROM messages
		WHERE from_email <> '' AND (lower(from_email) LIKE ? ESCAPE '\' OR lower(from_name) LIKE ? ESCAPE '\' OR lower(from_name) LIKE ? ESCAPE '\')
		GROUP BY lower(from_email) ORDER BY count(*) DESC, max(date) DESC LIMIT ?`,
		esc+"%", esc+"%", "% "+esc+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []mail.Address{}
	for rows.Next() {
		var a mail.Address
		if err := rows.Scan(&a.Name, &a.Email); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetFolderFloor records how far back a folder's continuous history goes.
func (s *Store) SetFolderFloor(folderID string, uid uint32) error {
	_, err := s.db.Exec(`UPDATE folders SET floor_uid = ? WHERE id = ?`, uid, folderID)
	return err
}

// ListFolderMessages returns one folder's messages, newest first, optionally
// filtered by a full-text query.
func (s *Store) ListFolderMessages(folderID, query string, limit int) ([]mail.MessageSummary, error) {
	where := []string{"m.folder_id = ?"}
	args := []any{folderID}
	from := "messages m"
	if fts := ftsQuery(query); fts != "" {
		from += " JOIN messages_fts ON messages_fts.rowid = m.rowid"
		where = append(where, "messages_fts MATCH ?")
		args = append(args, fts)
	}
	args = append(args, limit)
	rows, err := s.db.Query(`SELECT m.id, m.account_id, m.folder_id, m.from_name, m.from_email, m.to_json, m.subject,
		m.snippet, m.date, m.unread, m.starred, m.has_attachments
		FROM `+from+` WHERE `+strings.Join(where, " AND ")+` ORDER BY m.date DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []mail.MessageSummary{}
	for rows.Next() {
		var m mail.MessageSummary
		var to string
		var date int64
		if err := rows.Scan(&m.ID, &m.AccountID, &m.FolderID, &m.From.Name, &m.From.Email, &to, &m.Subject,
			&m.Snippet, &date, &m.Unread, &m.Starred, &m.HasAttachments); err != nil {
			return nil, err
		}
		m.Date = time.Unix(date, 0)
		_ = json.Unmarshal([]byte(to), &m.To)
		out = append(out, m)
	}
	return out, rows.Err()
}
