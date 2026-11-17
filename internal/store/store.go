// Package store is Pigeon's local mail database (SQLite). It caches accounts,
// folders and messages so the UI is instant and search works offline.
// Passwords and tokens are never stored here (see internal/secrets).
package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no CGO)
)

var memCounter atomic.Int64

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// DefaultPath is %AppData%\Pigeon\pigeon.db on Windows.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "Pigeon", "pigeon.db"), nil
}

// Open opens (and migrates) the database at path. Use ":memory:" for tests.
func Open(path string) (*Store, error) {
	// Each in-memory store gets its own named database so tests stay isolated.
	dsn := fmt.Sprintf("file:pigeonmem%d?mode=memory&cache=shared", memCounter.Add(1))
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		dsn = "file:" + filepath.ToSlash(path)
	}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	sep := "?"
	if path == ":memory:" {
		sep = "&"
	}
	db, err := sql.Open("sqlite", dsn+sep+q.Encode())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite has one writer; this avoids SQLITE_BUSY between goroutines
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

const schema = `
CREATE TABLE IF NOT EXISTS accounts (
	id            TEXT PRIMARY KEY,
	name          TEXT NOT NULL,
	email         TEXT NOT NULL,
	provider      TEXT NOT NULL,
	color         TEXT NOT NULL,
	username      TEXT NOT NULL DEFAULT '',
	imap_host     TEXT NOT NULL DEFAULT '',
	imap_port     INTEGER NOT NULL DEFAULT 993,
	imap_security TEXT NOT NULL DEFAULT 'tls',
	smtp_host     TEXT NOT NULL DEFAULT '',
	smtp_port     INTEGER NOT NULL DEFAULT 465,
	smtp_security TEXT NOT NULL DEFAULT 'tls',
	demo          INTEGER NOT NULL DEFAULT 0,
	auth          TEXT NOT NULL DEFAULT 'password',
	created_at    INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS folders (
	id          TEXT PRIMARY KEY,
	account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	path        TEXT NOT NULL,
	name        TEXT NOT NULL,
	role        TEXT NOT NULL,
	uidvalidity INTEGER NOT NULL DEFAULT 0,
	floor_uid   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS folders_account ON folders(account_id);

CREATE TABLE IF NOT EXISTS messages (
	id              TEXT PRIMARY KEY,
	account_id      TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	folder_id       TEXT NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
	uid             INTEGER NOT NULL DEFAULT 0,
	message_id      TEXT NOT NULL DEFAULT '',
	from_name       TEXT NOT NULL DEFAULT '',
	from_email      TEXT NOT NULL DEFAULT '',
	to_json         TEXT NOT NULL DEFAULT '[]',
	cc_json         TEXT NOT NULL DEFAULT '[]',
	subject         TEXT NOT NULL DEFAULT '',
	snippet         TEXT NOT NULL DEFAULT '',
	date            INTEGER NOT NULL DEFAULT 0,
	size            INTEGER NOT NULL DEFAULT 0,
	unread          INTEGER NOT NULL DEFAULT 0,
	starred         INTEGER NOT NULL DEFAULT 0,
	has_attachments INTEGER NOT NULL DEFAULT 0,
	body_html       TEXT NOT NULL DEFAULT '',
	body_text       TEXT NOT NULL DEFAULT '',
	body_fetched    INTEGER NOT NULL DEFAULT 0,
	parts_indexed   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS messages_folder_date ON messages(folder_id, date DESC);
CREATE INDEX IF NOT EXISTS messages_account ON messages(account_id);

-- Attachments and embedded images. Data is kept only for small inline images;
-- attachment bytes are fetched from the server when opened or saved.
CREATE TABLE IF NOT EXISTS parts (
	message_id   TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE ON UPDATE CASCADE,
	idx          INTEGER NOT NULL,
	filename     TEXT NOT NULL DEFAULT '',
	content_type TEXT NOT NULL DEFAULT '',
	size         INTEGER NOT NULL DEFAULT 0,
	content_id   TEXT NOT NULL DEFAULT '',
	inline       INTEGER NOT NULL DEFAULT 0,
	data         BLOB,
	PRIMARY KEY (message_id, idx)
);

-- Full-text search over everything a person would search for.
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
	subject, from_name, from_email, snippet, body_text,
	content='messages', content_rowid='rowid', tokenize='unicode61 remove_diacritics 2'
);
CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
	INSERT INTO messages_fts(rowid, subject, from_name, from_email, snippet, body_text)
	VALUES (new.rowid, new.subject, new.from_name, new.from_email, new.snippet, new.body_text);
END;
CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
	INSERT INTO messages_fts(messages_fts, rowid, subject, from_name, from_email, snippet, body_text)
	VALUES ('delete', old.rowid, old.subject, old.from_name, old.from_email, old.snippet, old.body_text);
END;
CREATE TRIGGER IF NOT EXISTS messages_au AFTER UPDATE OF subject, from_name, from_email, snippet, body_text ON messages BEGIN
	INSERT INTO messages_fts(messages_fts, rowid, subject, from_name, from_email, snippet, body_text)
	VALUES ('delete', old.rowid, old.subject, old.from_name, old.from_email, old.snippet, old.body_text);
	INSERT INTO messages_fts(rowid, subject, from_name, from_email, snippet, body_text)
	VALUES (new.rowid, new.subject, new.from_name, new.from_email, new.snippet, new.body_text);
END;
`

func (s *Store) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	// Columns added after the first release.
	if err := s.addColumn("accounts", "auth", "TEXT NOT NULL DEFAULT 'password'"); err != nil {
		return err
	}
	if err := s.addColumn("messages", "cc_json", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := s.addColumn("folders", "floor_uid", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	return s.addColumn("messages", "parts_indexed", "INTEGER NOT NULL DEFAULT 0")
}

func (s *Store) addColumn(table, column, def string) error {
	rows, err := s.db.Query("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	rows.Close()
	_, err = s.db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + def)
	return err
}
