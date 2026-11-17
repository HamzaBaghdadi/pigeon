package store

import (
	"testing"
	"time"

	"github.com/HamzaBaghdadi/pigeon/internal/mail"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	acct := Account{Account: mail.Account{ID: "a1", Name: "Test", Email: "me@example.com", Provider: mail.ProviderIMAP, Color: "#000"}}
	if err := s.SaveAccount(acct); err != nil {
		t.Fatal(err)
	}
	for _, f := range []struct {
		path string
		role mail.FolderRole
	}{{"INBOX", mail.RoleInbox}, {"Archive", mail.RoleArchive}} {
		id := FolderID("a1", f.path)
		if err := s.UpsertFolder(FolderRow{Folder: mail.Folder{ID: id, AccountID: "a1", Name: f.path, Role: f.role, Path: f.path}}); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func addMsg(t *testing.T, s *Store, uid uint32, subject, body string, unread bool) string {
	t.Helper()
	folder := FolderID("a1", "INBOX")
	id := MessageID(folder, uid)
	err := s.UpsertMessage(MessageRow{
		MessageSummary: mail.MessageSummary{ID: id, AccountID: "a1", FolderID: folder, Subject: subject,
			From: mail.Address{Name: "Lina Haddad", Email: "lina@example.com"}, Date: time.Unix(int64(1000+uid), 0), Unread: unread},
		UID: uid, BodyText: body,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestListAndSearch(t *testing.T) {
	s := newTestStore(t)
	addMsg(t, s, 1, "Q4 roadmap review", "decisions about search", true)
	addMsg(t, s, 2, "Lunch?", "tacos on friday", false)

	all, err := s.ListMessages("", mail.RoleInbox, "", 50)
	if err != nil || len(all) != 2 {
		t.Fatalf("ListMessages = %d, %v; want 2", len(all), err)
	}
	if all[0].Subject != "Lunch?" {
		t.Errorf("want newest first, got %q", all[0].Subject)
	}

	for q, want := range map[string]int{"road": 1, "tacos": 1, "lina": 2, "lin road": 1, `"quote`: 0, "nothing": 0} {
		got, err := s.ListMessages("", mail.RoleInbox, q, 50)
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		if len(got) != want {
			t.Errorf("search %q = %d results, want %d", q, len(got), want)
		}
	}
}

func TestFlagsMoveAndUnreadCounts(t *testing.T) {
	s := newTestStore(t)
	id := addMsg(t, s, 7, "Hello", "body", true)

	inbox, _ := s.GetFolder(FolderID("a1", "INBOX"))
	if inbox.Unread != 1 {
		t.Fatalf("unread = %d, want 1", inbox.Unread)
	}
	read := false
	if err := s.SetFlags(id, &read, nil); err != nil {
		t.Fatal(err)
	}
	inbox, _ = s.GetFolder(FolderID("a1", "INBOX"))
	if inbox.Unread != 0 {
		t.Fatalf("unread after read = %d, want 0", inbox.Unread)
	}

	newID, err := s.MoveLocal(id, FolderID("a1", "Archive"), 3)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListMessages("", mail.RoleArchive, "hello", 10); len(got) != 1 || got[0].ID != newID {
		t.Fatalf("moved message not found in archive search: %+v", got)
	}
}

func TestBodyKeptOnResync(t *testing.T) {
	s := newTestStore(t)
	id := addMsg(t, s, 1, "Hi", "", true)
	if err := s.SetBody(id, "<p>full</p>", "full text", "full text", false, nil); err != nil {
		t.Fatal(err)
	}
	addMsg(t, s, 1, "Hi", "", false) // a later sync refreshes flags only
	m, _ := s.GetMessage(id)
	if !m.BodyFetched || m.BodyText != "full text" || m.Unread {
		t.Errorf("resync clobbered body or missed flags: %+v", m)
	}
	if got, _ := s.ListMessages("", mail.RoleInbox, "full", 10); len(got) != 1 {
		t.Errorf("body not searchable after SetBody")
	}
}

func TestDeleteAccountCascades(t *testing.T) {
	s := newTestStore(t)
	addMsg(t, s, 1, "Hi", "", true)
	if err := s.DeleteAccount("a1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListMessages("", mail.RoleInbox, "", 10); len(got) != 0 {
		t.Errorf("messages survived account deletion")
	}
}

func TestMigratesOldAccountsTable(t *testing.T) {
	path := t.TempDir() + "/old.db"
	old, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a database from before the auth column existed.
	if _, err := old.db.Exec(`ALTER TABLE accounts DROP COLUMN auth`); err != nil {
		t.Fatal(err)
	}
	if _, err := old.db.Exec(`INSERT INTO accounts (id, name, email, provider, color) VALUES ('x','X','x@example.com','imap','#000')`); err != nil {
		t.Fatal(err)
	}
	old.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s.Close()
	a, err := s.GetAccount("x")
	if err != nil || a.Auth != AuthPassword {
		t.Fatalf("migrated account = %+v, %v", a, err)
	}
}

func TestCcAndContactSuggestions(t *testing.T) {
	s := newTestStore(t)
	folder := FolderID("a1", "INBOX")
	for i, from := range []mail.Address{
		{Name: "Lina Haddad", Email: "lina@example.com"},
		{Name: "Lina Haddad", Email: "lina@example.com"},
		{Name: "Omar Saleh", Email: "omar@example.com"},
		{Name: "100% Real", Email: "odd_name@example.com"},
	} {
		uid := uint32(i + 1)
		if err := s.UpsertMessage(MessageRow{MessageSummary: mail.MessageSummary{
			ID: MessageID(folder, uid), AccountID: "a1", FolderID: folder, From: from, Date: time.Unix(int64(uid), 0),
			Cc: []mail.Address{{Name: "Sara", Email: "sara@example.com"}},
		}, UID: uid}); err != nil {
			t.Fatal(err)
		}
	}
	m, _ := s.GetMessage(MessageID(folder, 1))
	if len(m.Cc) != 1 || m.Cc[0].Email != "sara@example.com" {
		t.Fatalf("cc not stored: %+v", m.Cc)
	}
	for q, want := range map[string]string{"lin": "lina@example.com", "hadd": "lina@example.com", "OMAR@": "omar@example.com", "100%": "odd_name@example.com"} {
		got, err := s.SuggestContacts(q, 5)
		if err != nil || len(got) != 1 || got[0].Email != want {
			t.Errorf("SuggestContacts(%q) = %+v, %v", q, got, err)
		}
	}
	if got, _ := s.SuggestContacts("%", 5); len(got) != 0 {
		t.Errorf("wildcard not escaped: %+v", got)
	}
}
