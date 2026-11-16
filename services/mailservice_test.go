package services

import (
	"strings"
	"testing"

	"github.com/HamzaBaghdadi/pigeon/internal/mail"
	"github.com/HamzaBaghdadi/pigeon/internal/settings"
	"github.com/HamzaBaghdadi/pigeon/internal/store"
)

func newTestServices(t *testing.T) (*MailService, *AccountService) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	prefs, err := settings.OpenAt(t.TempDir() + "/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	core := NewCore(st, prefs)
	return NewMailService(core), NewAccountService(core)
}

func TestSampleDataMoveUndoAndFlags(t *testing.T) {
	ms, as := newTestServices(t)
	if err := as.LoadSampleData(); err != nil {
		t.Fatal(err)
	}
	inbox, err := ms.ListMessages("", mail.RoleInbox, "")
	if err != nil || len(inbox) != 8 {
		t.Fatalf("inbox = %d, %v; want 8", len(inbox), err)
	}

	// The sanitizer runs on read: no scripts, remote images counted.
	var digest mail.MessageSummary
	for _, m := range inbox {
		if strings.HasPrefix(m.Subject, "This week in open source") {
			digest = m
		}
	}
	body, err := ms.GetMessage(digest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body.HTML, "<script") || strings.Contains(body.HTML, "onclick=") || body.RemoteImages != 2 {
		t.Errorf("unsafe or wrong body: images=%d html=%s", body.RemoteImages, body.HTML)
	}

	// Archive, then undo with the returned ID.
	res, err := ms.Move(digest.ID, mail.RoleArchive)
	if err != nil || res.ID == "" || res.From != mail.RoleInbox {
		t.Fatalf("Move = %+v, %v", res, err)
	}
	if got, _ := ms.ListMessages("", mail.RoleArchive, ""); len(got) != 1 {
		t.Fatalf("archive = %d, want 1", len(got))
	}
	if _, err := ms.Move(res.ID, res.From); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if got, _ := ms.ListMessages("", mail.RoleInbox, ""); len(got) != 8 {
		t.Fatalf("inbox after undo = %d, want 8", len(got))
	}

	// Flags and unread counts.
	unreadBefore := countUnread(t, ms)
	if err := ms.SetRead(inbox[0].ID, inbox[0].Unread); err != nil {
		t.Fatal(err)
	}
	if got := countUnread(t, ms); got == unreadBefore {
		t.Errorf("unread count didn't change (%d)", got)
	}

	// Search runs over bodies too.
	if got, _ := ms.ListMessages("", mail.RoleInbox, "keychain"); len(got) != 1 {
		t.Errorf("body search = %d, want 1", len(got))
	}

	if err := as.RemoveSampleData(); err != nil {
		t.Fatal(err)
	}
	if accts, _ := ms.ListAccounts(); len(accts) != 0 {
		t.Errorf("accounts after removing samples = %d", len(accts))
	}
}

func countUnread(t *testing.T, ms *MailService) int {
	folders, err := ms.ListFolders("")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range folders {
		if f.Role == mail.RoleInbox {
			n += f.Unread
		}
	}
	return n
}

func TestTextToHTMLLinksAndEscapes(t *testing.T) {
	got := textToHTML("See https://example.com/a?b=1. <script>")
	if !strings.Contains(got, `<a href="https://example.com/a?b=1">`) || strings.Contains(got, "<script>") {
		t.Errorf("textToHTML = %s", got)
	}
}

func TestGoogleSetupBuiltInAndOverride(t *testing.T) {
	_, as := newTestServices(t)
	oldID, oldSecret := GoogleClientID, GoogleClientSecret
	t.Cleanup(func() { GoogleClientID, GoogleClientSecret = oldID, oldSecret })

	GoogleClientID, GoogleClientSecret = "", ""
	if s := as.GetGoogleSetup(); s.Configured {
		t.Fatalf("no client anywhere, but Configured=true: %+v", s)
	}

	GoogleClientID, GoogleClientSecret = "111-release.apps.googleusercontent.com", "x"
	if s := as.GetGoogleSetup(); !s.Configured || !s.BuiltIn {
		t.Fatalf("release build should use the built-in client: %+v", s)
	}
	if c := as.core.google.client(); c.ID != GoogleClientID {
		t.Fatalf("sign-in would use %q, want the built-in client", c.ID)
	}

	// A user's own client (from Settings) takes precedence.
	if _, err := as.core.prefs.Update(func(v *settings.Settings) { v.GoogleClientID = "222-mine.apps.googleusercontent.com" }); err != nil {
		t.Fatal(err)
	}
	if s := as.GetGoogleSetup(); s.BuiltIn || s.ClientID != "222-mine.apps.googleusercontent.com" {
		t.Fatalf("user client should win: %+v", s)
	}
}
