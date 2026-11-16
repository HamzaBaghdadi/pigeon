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
