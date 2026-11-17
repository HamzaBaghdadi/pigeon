package services

import (
	"strings"
	"testing"
	"time"

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

func TestComposeDraftsAndDemoSend(t *testing.T) {
	ms, as := newTestServices(t)
	cs := NewComposeService(as.core)
	if err := as.LoadSampleData(); err != nil {
		t.Fatal(err)
	}
	inbox, _ := ms.ListMessages("demo-work", mail.RoleInbox, "roadmap")
	if len(inbox) != 1 {
		t.Fatalf("want Lina's roadmap message, got %d", len(inbox))
	}
	lina := inbox[0]

	reply, err := cs.NewDraft(lina.ID, ModeReply)
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.To) != 1 || reply.To[0].Email != "lina@work.example" || reply.Subject != "Re: "+lina.Subject {
		t.Errorf("reply draft = %+v", reply)
	}
	if !strings.Contains(reply.Body, "Lina Haddad <lina@work.example> wrote:") || !strings.Contains(reply.Body, "> Hi team,") {
		t.Errorf("reply quote = %q", reply.Body)
	}

	fwd, err := cs.NewDraft(lina.ID, ModeForward)
	if err != nil {
		t.Fatal(err)
	}
	if len(fwd.To) != 0 || !strings.HasPrefix(fwd.Subject, "Fwd: ") || !strings.Contains(fwd.Body, "Forwarded message") {
		t.Errorf("forward draft = %+v", fwd)
	}

	if _, err := as.core.prefs.Update(func(v *settings.Settings) { v.DisplayName = "Hamza Baghdadi" }); err != nil {
		t.Fatal(err)
	}
	reply.Body = "Thanks Lina!" + reply.Body
	if err := cs.Send(reply); err != nil {
		t.Fatalf("Send: %v", err)
	}
	sent, _ := ms.ListMessages("demo-work", mail.RoleSent, "")
	if len(sent) != 1 || sent[0].Subject != "Re: "+lina.Subject || !strings.HasPrefix(sent[0].Snippet, "Thanks Lina!") || sent[0].From.Name != "Hamza Baghdadi" {
		t.Fatalf("sent folder = %+v", sent)
	}

	if err := cs.Send(Draft{AccountID: "demo-work", Subject: "x"}); err == nil {
		t.Error("sent a message with no recipients")
	}
	if err := cs.Send(Draft{AccountID: "demo-work", To: []mail.Address{{Email: "a@example.com"}}, Attachments: []AttachmentInfo{{ID: "forged", Name: "C:/Windows/win.ini"}}}); err == nil {
		t.Error("accepted an attachment ticket that wasn't issued by the file picker")
	}
}

func TestReplyAllExcludesSelf(t *testing.T) {
	ms, as := newTestServices(t)
	cs := NewComposeService(as.core)
	_ = as.LoadSampleData()
	st := as.core.store
	folder := store.FolderID("demo-work", "Inbox")
	if err := st.UpsertMessage(store.MessageRow{MessageSummary: mail.MessageSummary{
		ID: store.MessageID(folder, 999), AccountID: "demo-work", FolderID: folder,
		From:    mail.Address{Name: "Omar", Email: "omar@work.example"},
		To:      []mail.Address{{Email: "me@work.example"}, {Email: "sara@work.example"}},
		Cc:      []mail.Address{{Email: "OMAR@work.example"}, {Email: "nour@work.example"}},
		Subject: "Team lunch", Date: time.Now(),
	}, UID: 999, BodyText: "Friday?", BodyFetched: true}); err != nil {
		t.Fatal(err)
	}
	_ = ms
	d, err := cs.NewDraft(store.MessageID(folder, 999), ModeReplyAll)
	if err != nil {
		t.Fatal(err)
	}
	var cc []string
	for _, a := range d.Cc {
		cc = append(cc, a.Email)
	}
	if len(d.To) != 1 || d.To[0].Email != "omar@work.example" || strings.Join(cc, ",") != "sara@work.example,nour@work.example" {
		t.Errorf("reply-all to=%v cc=%v", d.To, cc)
	}
}

func TestFoldersMoveToFolderAndUndo(t *testing.T) {
	ms, as := newTestServices(t)
	_ = as.LoadSampleData()
	receipts := store.FolderID("demo-personal", "Receipts")
	travel := store.FolderID("demo-personal", "Travel")

	got, err := ms.ListFolderMessages(receipts, "")
	if err != nil || len(got) != 1 || got[0].Subject != "Your receipt from Coffee Corner" {
		t.Fatalf("receipts folder = %+v, %v", got, err)
	}
	if got, _ := ms.ListFolderMessages(travel, "lisbon"); len(got) != 1 {
		t.Fatalf("search within folder = %d, want 1", len(got))
	}

	// Move a receipt into Travel, then undo back to Receipts.
	res, err := ms.MoveToFolder(got[0].ID, receipts)
	if err != nil {
		t.Fatal(err)
	}
	_ = res
	rec, _ := ms.ListFolderMessages(receipts, "")
	moved, err := ms.MoveToFolder(rec[0].ID, travel)
	if err != nil || moved.FromFolder != receipts {
		t.Fatalf("MoveToFolder = %+v, %v", moved, err)
	}
	if l, _ := ms.ListFolderMessages(travel, ""); len(l) != 2 {
		t.Fatalf("travel after move = %d, want 2", len(l))
	}
	if _, err := ms.MoveToFolder(moved.ID, moved.FromFolder); err != nil {
		t.Fatal(err)
	}
	if l, _ := ms.ListFolderMessages(receipts, ""); len(l) != 1 {
		t.Fatalf("receipts after undo = %d, want 1", len(l))
	}

	// Can't move across accounts.
	if _, err := ms.MoveToFolder(rec[0].ID, store.FolderID("demo-side", "Pigeon/Design")); err == nil {
		t.Error("moved a message into another account's folder")
	}

	// Sample accounts have no server history to page.
	if r, err := ms.LoadOlder("", mail.RoleInbox, ""); err != nil || !r.Done || r.Added != 0 {
		t.Errorf("LoadOlder on samples = %+v, %v", r, err)
	}
}
