package imapsync

import (
	"context"
	"fmt"
	"net"
	"strings"
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

func TestXOAuth2InitialResponse(t *testing.T) {
	mech, ir, err := newXOAuth2Client("me@gmail.com", "test-access-token").Start()
	if err != nil || mech != "XOAUTH2" {
		t.Fatalf("Start = %s, %v", mech, err)
	}
	if want := "user=me@gmail.comauth=Bearer test-access-token"; string(ir) != want {
		t.Errorf("initial response = %q, want %q", ir, want)
	}
}
