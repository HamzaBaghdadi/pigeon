package services

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/HamzaBaghdadi/pigeon/internal/mail"
	"github.com/HamzaBaghdadi/pigeon/internal/store"
)

func TestSafeFilenameAndDanger(t *testing.T) {
	cases := map[string]string{
		`..\..\Windows\evil.exe`: "evil.exe",
		"report.pdf. . ":         "report.pdf",
		"a/b:c*d?.txt":           "b_c_d_.txt",
		"":                       "attachment-3",
	}
	for in, want := range cases {
		if got := safeFilename(in, 2); got != want {
			t.Errorf("safeFilename(%q) = %q, want %q", in, got, want)
		}
	}
	for name, want := range map[string]bool{"setup.EXE": true, "invoice.pdf.js": true, "photo.jpg": false, "notes.txt": false, "x.lnk": true, "x.iso": true} {
		if got := IsDangerous(name); got != want {
			t.Errorf("IsDangerous(%q) = %v", name, got)
		}
	}
}

func TestGetMessageAttachmentsAndInlineImages(t *testing.T) {
	ms, as := newTestServices(t)
	_ = as.LoadSampleData()
	st := as.core.store
	folder := store.FolderID("demo-work", "Inbox")
	id := store.MessageID(folder, 777)
	if err := st.UpsertMessage(store.MessageRow{MessageSummary: mail.MessageSummary{
		ID: id, AccountID: "demo-work", FolderID: folder, Subject: "Logo", Date: time.Now(), HasAttachments: true,
	}, UID: 777}); err != nil {
		t.Fatal(err)
	}
	html := `<p><img src="cid:logo@x" alt="logo"> <img src="cid:vector@x"></p>`
	if err := st.SetBody(id, html, "Logo", "Logo", true, []store.Part{
		{Attachment: mail.Attachment{Index: 0, ContentType: "image/png", ContentID: "logo@x", Inline: true, Size: 4}, Data: []byte{0x89, 'P', 'N', 'G'}},
		{Attachment: mail.Attachment{Index: 1, ContentType: "image/svg+xml", ContentID: "vector@x", Inline: true, Size: 10}, Data: []byte("<svg></svg>")},
		{Attachment: mail.Attachment{Index: 2, Filename: "q4.csv", ContentType: "text/csv", Size: 20}},
	}); err != nil {
		t.Fatal(err)
	}
	body, err := ms.GetMessage(id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.HTML, `src="data:image/png;base64,iVBORw==`) {
		t.Errorf("inline PNG not embedded: %s", body.HTML)
	}
	if strings.Contains(body.HTML, "data:image/svg") {
		t.Error("SVG must never be inlined (it can carry script)")
	}
	if body.RemoteImages != 0 {
		t.Errorf("embedded images counted as remote: %d", body.RemoteImages)
	}
	if len(body.Attachments) != 1 || body.Attachments[0].Filename != "q4.csv" {
		t.Errorf("attachments = %+v", body.Attachments)
	}
	// Moving the message keeps its parts (the parts table follows the ID).
	res, err := ms.Move(id, mail.RoleArchive)
	if err != nil {
		t.Fatal(err)
	}
	if parts, _ := st.ListParts(res.ID); len(parts) != 3 {
		t.Errorf("parts after move = %d, want 3", len(parts))
	}
	// Sample messages have no real files to open.
	if err := ms.OpenAttachment(res.ID, 2); err == nil {
		t.Error("opened an attachment of a sample message")
	}
}

func TestMarkOfTheWeb(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	p := filepath.Join(t.TempDir(), "invoice.pdf")
	if err := os.WriteFile(p, []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	markDownloaded(p)
	zone, err := os.ReadFile(p + ":Zone.Identifier")
	if err != nil || !strings.Contains(string(zone), "ZoneId=3") {
		t.Fatalf("Zone.Identifier = %q, %v", zone, err)
	}
}
