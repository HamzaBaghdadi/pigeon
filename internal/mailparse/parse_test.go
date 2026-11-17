package mailparse

import (
	"strings"
	"testing"
)

const multipart = "From: Lina <lina@example.com>\r\n" +
	"To: me@example.com\r\n" +
	"Subject: =?UTF-8?Q?Caf=C3=A9_plans?=\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/mixed; boundary=outer\r\n\r\n" +
	"--outer\r\n" +
	"Content-Type: multipart/alternative; boundary=alt\r\n\r\n" +
	"--alt\r\n" +
	"Content-Type: text/plain; charset=iso-8859-1\r\n" +
	"Content-Transfer-Encoding: quoted-printable\r\n\r\n" +
	"Rendez-vous au caf=E9 =E0 10h.\r\n\r\nOn Mon, Nov 16, 2026 at 9:00 AM Omar wrote:\r\n> earlier\r\n" +
	"--alt\r\n" +
	"Content-Type: text/html; charset=utf-8\r\n\r\n" +
	"<p>Rendez-vous au <b>café</b></p>\r\n" +
	"--alt--\r\n" +
	"--outer\r\n" +
	"Content-Type: application/pdf\r\n" +
	"Content-Disposition: attachment; filename=plan.pdf\r\n" +
	"Content-Transfer-Encoding: base64\r\n\r\n" +
	"JVBERi0=\r\n" +
	"--outer--\r\n"

func TestParseMultipart(t *testing.T) {
	b, err := Parse([]byte(multipart))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.Text, "café à 10h") {
		t.Errorf("charset not decoded: %q", b.Text)
	}
	if !strings.Contains(b.HTML, "<b>café</b>") {
		t.Errorf("html part missing: %q", b.HTML)
	}
	if !b.HasAttachments {
		t.Error("attachment not detected")
	}
	if b.Snippet != "Rendez-vous au café à 10h." {
		t.Errorf("snippet = %q", b.Snippet)
	}
}

func TestHTMLOnly(t *testing.T) {
	raw := "Subject: x\r\nContent-Type: text/html\r\n\r\n<style>p{}</style><p>Hello&nbsp;<i>there</i></p><div>second</div>"
	b, _ := Parse([]byte(raw))
	if b.Snippet != "Hello there second" {
		t.Errorf("snippet = %q", b.Snippet)
	}
}

const withInline = "From: a@example.com\r\nSubject: logo\r\nMIME-Version: 1.0\r\n" +
	"Content-Type: multipart/mixed; boundary=mix\r\n\r\n" +
	"--mix\r\nContent-Type: multipart/related; boundary=rel\r\n\r\n" +
	"--rel\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Hi <img src=\"cid:logo@pigeon\"></p>\r\n" +
	"--rel\r\nContent-Type: image/png\r\nContent-ID: <logo@pigeon>\r\nContent-Transfer-Encoding: base64\r\n\r\niVBORw0KGgo=\r\n" +
	"--rel--\r\n" +
	"--mix\r\nContent-Type: text/csv; name=\"report.csv\"\r\nContent-Disposition: attachment; filename=\"report.csv\"\r\n\r\na,b\r\n1,2\r\n" +
	"--mix--\r\n"

func TestPartsAndInlineImages(t *testing.T) {
	b, err := Parse([]byte(withInline))
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Parts) != 2 {
		t.Fatalf("parts = %+v", b.Parts)
	}
	img, att := b.Parts[0], b.Parts[1]
	if !img.Inline || img.ContentID != "logo@pigeon" || img.ContentType != "image/png" || len(img.Data) == 0 {
		t.Errorf("inline image = %+v", img)
	}
	if att.Inline || att.Filename != "report.csv" || att.Index != 1 || att.Data != nil {
		t.Errorf("attachment = %+v", att)
	}
	if !b.HasAttachments {
		t.Error("HasAttachments should be true (report.csv)")
	}
	p, data, err := PartData([]byte(withInline), 1)
	if err != nil || p.Filename != "report.csv" || !strings.Contains(string(data), "1,2") {
		t.Errorf("PartData = %+v %q %v", p, data, err)
	}
	if _, _, err := PartData([]byte(withInline), 5); err == nil {
		t.Error("PartData accepted a missing index")
	}
}
