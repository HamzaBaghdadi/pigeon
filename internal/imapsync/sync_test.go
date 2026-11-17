package imapsync

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"github.com/HamzaBaghdadi/pigeon/internal/mail"
	"github.com/HamzaBaghdadi/pigeon/internal/store"
)

const (
	testUser = "me@example.com"
	testPass = "app-password"
)

// startServer runs an in-memory IMAP server with INBOX, Sent (special-use)
// and Archive (name only) and returns its address.
func startServer(t *testing.T) string {
	t.Helper()
	mem := imapmemserver.New()
	u := imapmemserver.NewUser(testUser, testPass)
	u.Create("INBOX", nil)
	u.Create("Sent", &imap.CreateOptions{SpecialUse: []imap.MailboxAttr{imap.MailboxAttrSent}})
	u.Create("Archive", nil)
	mem.AddUser(u)

	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		InsecureAuth: true,
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapIMAP4rev2: {}, imap.CapSpecialUse: {}},
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	dialInsecure = true
	t.Cleanup(func() { dialInsecure = false })
	return ln.Addr().String()
}

func rawMessage(i int) string {
	return fmt.Sprintf("From: Sender %d <s%d@example.com>\r\nTo: %s\r\nSubject: Message %d\r\n"+
		"Date: Mon, 16 Nov 2026 10:%02d:00 +0000\r\nMessage-ID: <m%d@example.com>\r\nContent-Type: text/plain\r\n\r\n"+
		"Hello number %d, the quarterly roadmap is attached.\r\n", i, i, testUser, i, i, i, i)
}

func appendMsgs(t *testing.T, addr string, n int, flags ...imap.Flag) {
	t.Helper()
	c, err := imapclient.DialInsecure(addr, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Login(testUser, testPass).Wait(); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= n; i++ {
		raw := rawMessage(i)
		cmd := c.Append("INBOX", int64(len(raw)), &imap.AppendOptions{Flags: flags})
		cmd.Write([]byte(raw))
		cmd.Close()
		if _, err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
}

func setup(t *testing.T) (string, *store.Store, *Engine, store.Account) {
	addr := startServer(t)
	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	fmt.Sscan(portStr, &port)

	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	acct := store.Account{
		Account:  mail.Account{ID: "acc", Name: "Test", Email: testUser, Provider: mail.ProviderIMAP, Color: "#000"},
		IMAPHost: host, IMAPPort: port, IMAPSecurity: "tls",
	}
	if err := st.SaveAccount(acct); err != nil {
		t.Fatal(err)
	}
	eng := NewEngine(st, func(store.Account) (Credentials, error) { return Credentials{Password: testPass}, nil })
	t.Cleanup(func() { eng.Disconnect("acc") })
	return addr, st, eng, acct
}

func TestVerify(t *testing.T) {
	addr := startServer(t)
	host, port, _ := net.SplitHostPort(addr)
	var p int
	fmt.Sscan(port, &p)
	if err := Verify(Config{Host: host, Port: p, Username: testUser, Password: testPass}); err != nil {
		t.Fatalf("Verify with right password: %v", err)
	}
	if err := Verify(Config{Host: host, Port: p, Username: testUser, Password: "wrong"}); err == nil {
		t.Fatal("Verify with wrong password succeeded")
	}
}

func TestSyncFlagsMoveAndDeletion(t *testing.T) {
	addr, st, eng, acct := setup(t)
	appendMsgs(t, addr, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := eng.SyncAccount(ctx, acct); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Folder roles: special-use attribute (Sent) and name guess (Archive).
	for _, role := range []mail.FolderRole{mail.RoleInbox, mail.RoleSent, mail.RoleArchive} {
		if _, err := st.FolderByRole("acc", role); err != nil {
			t.Errorf("no folder with role %s: %v", role, err)
		}
	}

	inbox, err := st.ListMessages("acc", mail.RoleInbox, "", 100)
	if err != nil || len(inbox) != 3 {
		t.Fatalf("inbox = %d messages, %v; want 3", len(inbox), err)
	}
	if !inbox[0].Unread || inbox[0].Subject != "Message 3" || inbox[0].From.Email != "s3@example.com" {
		t.Errorf("unexpected newest message: %+v", inbox[0])
	}
	// Bodies were prefetched and are searchable offline.
	if got, _ := st.ListMessages("acc", mail.RoleInbox, "quarterly roadmap", 100); len(got) != 3 {
		t.Errorf("full-text search found %d, want 3", len(got))
	}
	full, _ := st.GetMessage(inbox[0].ID)
	if !full.BodyFetched || !strings.Contains(full.Snippet, "Hello number 3") {
		t.Errorf("body not prefetched: %+v", full)
	}

	// Mark read on the server, resync, and the flag must stick.
	if err := eng.SetFlag(acct, full, imap.FlagSeen, true); err != nil {
		t.Fatalf("SetFlag: %v", err)
	}
	if err := eng.SyncAccount(ctx, acct); err != nil {
		t.Fatal(err)
	}
	if m, _ := st.GetMessage(full.ID); m.Unread {
		t.Error("message still unread after \\Seen + resync")
	}

	// Archive: MOVE reports the new UID, so the local row follows the message.
	archive, _ := st.FolderByRole("acc", mail.RoleArchive)
	newID, err := eng.Move(acct, full, archive)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if newID == "" {
		t.Fatal("Move didn't return the new ID")
	}
	if got, _ := st.ListMessages("acc", mail.RoleArchive, "", 10); len(got) != 1 || got[0].ID != newID {
		t.Fatalf("archive after move = %+v", got)
	}
	if err := eng.SyncAccount(ctx, acct); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.ListMessages("acc", mail.RoleInbox, "", 10); len(got) != 2 {
		t.Errorf("inbox after move+resync = %d, want 2", len(got))
	}
	if got, _ := st.ListMessages("acc", mail.RoleArchive, "", 10); len(got) != 1 {
		t.Errorf("archive after resync = %d, want 1", len(got))
	}

	// Deleted elsewhere (another client expunges) -> removed locally on next sync.
	other, _ := imapclient.DialInsecure(addr, nil)
	defer other.Close()
	other.Login(testUser, testPass).Wait()
	other.Select("INBOX", nil).Wait()
	other.Store(imap.SeqSetNum(1), &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Close()
	other.Expunge().Close()
	if err := eng.SyncAccount(ctx, acct); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.ListMessages("acc", mail.RoleInbox, "", 10); len(got) != 1 {
		t.Errorf("inbox after remote delete = %d, want 1", len(got))
	}
}

func TestAppendSent(t *testing.T) {
	_, st, eng, acct := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := eng.SyncAccount(ctx, acct); err != nil {
		t.Fatal(err)
	}
	raw := []byte("From: me@example.com\r\nTo: lina@example.com\r\nSubject: Sent from Pigeon\r\nDate: Mon, 16 Nov 2026 11:00:00 +0000\r\n\r\nHello Lina\r\n")
	if err := eng.AppendSent(acct, raw); err != nil {
		t.Fatalf("AppendSent: %v", err)
	}
	if err := eng.SyncAccount(ctx, acct); err != nil {
		t.Fatal(err)
	}
	sent, _ := st.ListMessages("acc", mail.RoleSent, "", 10)
	if len(sent) != 1 || sent[0].Subject != "Sent from Pigeon" || sent[0].Unread {
		t.Fatalf("sent folder = %+v", sent)
	}
}

func TestIdlePushAndNewMailDetection(t *testing.T) {
	addr, _, eng, acct := setup(t)
	appendMsgs(t, addr, 2)

	var mu sync.Mutex
	var reported []store.MessageRow
	eng.OnNewMail = func(_ store.Account, msgs []store.MessageRow) {
		mu.Lock()
		reported = append(reported, msgs...)
		mu.Unlock()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First sync of an account: existing mail is not "new".
	if err := eng.SyncAccount(ctx, acct); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if len(reported) != 0 {
		t.Fatalf("first sync reported %d messages as new", len(reported))
	}
	mu.Unlock()

	// Watch the inbox; on a push, sync just the inbox.
	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	go eng.Watch(watchCtx, acct, func() { _ = eng.SyncInbox(watchCtx, acct) })
	time.Sleep(300 * time.Millisecond) // let IDLE start

	// Mail arrives from elsewhere.
	c, err := imapclient.DialInsecure(addr, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Login(testUser, testPass).Wait()
	raw := "From: Nour <nour@example.org>\r\nTo: " + testUser + "\r\nSubject: Pushed!\r\nContent-Type: text/plain\r\n\r\nArrived via IDLE.\r\n"
	cmd := c.Append("INBOX", int64(len(raw)), nil)
	cmd.Write([]byte(raw))
	cmd.Close()
	if _, err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(reported)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reported) != 1 || reported[0].Subject != "Pushed!" || reported[0].From.Email != "nour@example.org" {
		t.Fatalf("pushed mail not reported correctly: %+v", reported)
	}
	if !strings.Contains(reported[0].Snippet, "Arrived via IDLE") {
		t.Errorf("new-mail report should include a preview, got %q", reported[0].Snippet)
	}
}

func TestLoadOlderKeepsHistoryAndServerSearch(t *testing.T) {
	addr, st, eng, acct := setup(t)
	old := windowByRole[mail.RoleInbox]
	windowByRole[mail.RoleInbox] = 5
	t.Cleanup(func() { windowByRole[mail.RoleInbox] = old })

	// An old message with a unique word, then 12 newer ones.
	c, _ := imapclient.DialInsecure(addr, nil)
	defer c.Close()
	c.Login(testUser, testPass).Wait()
	zebra := "From: Zed <zed@example.org>\r\nTo: " + testUser + "\r\nSubject: Field notes\r\nContent-Type: text/plain\r\n\r\nWe saw a zebra today.\r\n"
	cmd := c.Append("INBOX", int64(len(zebra)), nil)
	cmd.Write([]byte(zebra))
	cmd.Close()
	cmd.Wait()
	appendMsgs(t, addr, 12)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := eng.SyncAccount(ctx, acct); err != nil {
		t.Fatal(err)
	}
	inbox, _ := st.FolderByRole("acc", mail.RoleInbox)
	count := func() int { l, _ := st.ListMessages("acc", mail.RoleInbox, "", 1000); return len(l) }
	if n := count(); n != 5 {
		t.Fatalf("after first sync: %d messages, want window of 5", n)
	}

	// Server search finds the old message that isn't downloaded yet.
	matched, err := eng.SearchServer(ctx, acct, inbox.ID, "zebra", 50)
	if err != nil || matched != 1 {
		t.Fatalf("SearchServer = %d, %v", matched, err)
	}
	if got, _ := st.ListMessages("acc", mail.RoleInbox, "zebra", 10); len(got) != 1 || got[0].Subject != "Field notes" {
		t.Fatalf("server result not searchable locally: %+v", got)
	}

	// Page through older mail until it's all loaded.
	if n, err := eng.LoadOlder(ctx, acct, inbox.ID, 4); err != nil || n != 4 {
		t.Fatalf("LoadOlder(4) = %d, %v", n, err)
	}
	for {
		n, err := eng.LoadOlder(ctx, acct, inbox.ID, 4)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
	}
	if n := count(); n != 13 {
		t.Fatalf("after loading all history: %d, want 13", n)
	}

	// A regular sync must keep the loaded history...
	if err := eng.SyncAccount(ctx, acct); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 13 {
		t.Fatalf("sync dropped loaded history: %d, want 13", n)
	}
	// ...but still notice old mail deleted or read elsewhere.
	c.Select("INBOX", nil).Wait()
	c.Store(imap.SeqSetNum(1), &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Close()
	c.Expunge().Close()
	c.Store(imap.SeqSetNum(1), &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagSeen}}, nil).Close()
	if err := eng.SyncAccount(ctx, acct); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 12 {
		t.Fatalf("remote delete of old mail not applied: %d, want 12", n)
	}
	if got, _ := st.ListMessages("acc", mail.RoleInbox, "zebra", 10); len(got) != 0 {
		t.Errorf("deleted old message still present")
	}
	all, _ := st.ListMessages("acc", mail.RoleInbox, "", 100)
	oldest := all[len(all)-1]
	if oldest.Unread {
		t.Errorf("flag change on old mail not applied: %+v", oldest)
	}
}

func TestAttachmentsIndexedAndFetchRaw(t *testing.T) {
	addr, st, eng, acct := setup(t)
	raw := "From: a@example.com\r\nTo: " + testUser + "\r\nSubject: Report\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=mix\r\n\r\n" +
		"--mix\r\nContent-Type: multipart/related; boundary=rel\r\n\r\n" +
		"--rel\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Logo <img src=\"cid:logo@x\"></p>\r\n" +
		"--rel\r\nContent-Type: image/png\r\nContent-ID: <logo@x>\r\nContent-Transfer-Encoding: base64\r\n\r\niVBORw0KGgo=\r\n" +
		"--rel--\r\n" +
		"--mix\r\nContent-Type: text/csv\r\nContent-Disposition: attachment; filename=\"q4.csv\"\r\n\r\nregion,total\r\nEMEA,42\r\n" +
		"--mix--\r\n"
	c, _ := imapclient.DialInsecure(addr, nil)
	defer c.Close()
	c.Login(testUser, testPass).Wait()
	cmd := c.Append("INBOX", int64(len(raw)), nil)
	cmd.Write([]byte(raw))
	cmd.Close()
	cmd.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := eng.SyncAccount(ctx, acct); err != nil {
		t.Fatal(err)
	}
	list, _ := st.ListMessages("acc", mail.RoleInbox, "", 10)
	if len(list) != 1 || !list[0].HasAttachments {
		t.Fatalf("message = %+v", list)
	}
	parts, _ := st.ListParts(list[0].ID)
	if len(parts) != 2 || !parts[0].Inline || parts[1].Filename != "q4.csv" {
		t.Fatalf("parts = %+v", parts)
	}
	imgs, _ := st.InlineImages(list[0].ID)
	if len(imgs["logo@x"].Data) == 0 {
		t.Fatalf("inline image data not stored: %+v", imgs)
	}
	m, _ := st.GetMessage(list[0].ID)
	got, err := eng.FetchRaw(acct, m)
	if err != nil || !strings.Contains(string(got), "EMEA,42") {
		t.Fatalf("FetchRaw = %d bytes, %v", len(got), err)
	}
	// Fetching raw content must not mark the message read.
	if m, _ := st.GetMessage(list[0].ID); !m.Unread {
		t.Error("FetchRaw marked the message as read")
	}
}
