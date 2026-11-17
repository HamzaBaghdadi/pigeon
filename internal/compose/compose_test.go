package compose

import (
	"strings"
	"testing"
	"time"

	gomail "github.com/emersion/go-message/mail"

	pmail "github.com/HamzaBaghdadi/pigeon/internal/mail"
	"github.com/HamzaBaghdadi/pigeon/internal/mailparse"
)

func sample() Message {
	return Message{
		From:        pmail.Address{Name: "Hamza", Email: "me@example.com"},
		To:          []pmail.Address{{Name: "Lina Haddad", Email: "lina@example.com"}},
		Cc:          []pmail.Address{{Email: "omar@example.com"}},
		Bcc:         []pmail.Address{{Email: "secret@example.com"}},
		Subject:     "Café plans\r\nBcc: injected@evil.example",
		Text:        "Hi Lina,\nSee https://example.com/plan.\n\n> earlier line",
		InReplyTo:   "<orig@example.com>",
		References:  []string{"<root@example.com>"},
		Attachments: []Attachment{{Name: `..\..\plan.pdf`, Data: []byte("%PDF-1.4 fake")}},
		Date:        time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC),
	}
}

func TestBuildRoundTrip(t *testing.T) {
	raw, id, err := Build(sample())
	if err != nil {
		t.Fatal(err)
	}
	if id == "" || !strings.HasSuffix(id, "@example.com") {
		t.Errorf("message id = %q", id)
	}
	s := string(raw)
	if strings.Contains(strings.ToLower(s), "secret@example.com") {
		t.Error("Bcc recipient leaked into the message")
	}
	if strings.Contains(s, "\r\nBcc: injected") {
		t.Error("header injection through the subject")
	}

	r, err := gomail.CreateReader(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	subj, _ := r.Header.Subject()
	if subj != "Café plans Bcc: injected@evil.example" {
		t.Errorf("subject = %q", subj)
	}
	irt, _ := r.Header.MsgIDList("In-Reply-To")
	refs, _ := r.Header.MsgIDList("References")
	if len(irt) != 1 || irt[0] != "orig@example.com" || len(refs) != 2 || refs[1] != "orig@example.com" {
		t.Errorf("threading headers: in-reply-to=%v references=%v", irt, refs)
	}

	body, err := mailparse.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.Text, "See https://example.com/plan.") {
		t.Errorf("text part = %q", body.Text)
	}
	if !strings.Contains(body.HTML, `<a href="https://example.com/plan">`) || !strings.Contains(body.HTML, "<blockquote") {
		t.Errorf("html part = %q", body.HTML)
	}
	if !body.HasAttachments || !strings.Contains(s, `filename=plan.pdf`) {
		t.Errorf("attachment missing or unsafe name")
	}
}

func TestRecipientsDeduplicated(t *testing.T) {
	m := sample()
	m.Cc = append(m.Cc, pmail.Address{Email: "LINA@example.com"})
	got := m.Recipients()
	if len(got) != 3 {
		t.Errorf("recipients = %v", got)
	}
}

func TestValidate(t *testing.T) {
	m := sample()
	m.To, m.Cc, m.Bcc = nil, nil, nil
	if err := m.Validate(); err == nil {
		t.Error("no recipients accepted")
	}
	m = sample()
	m.To = []pmail.Address{{Email: "not an address"}}
	if err := m.Validate(); err == nil {
		t.Error("invalid address accepted")
	}
	m = sample()
	m.Attachments = []Attachment{{Name: "big.bin", Data: make([]byte, MaxAttachments+1)}}
	if err := m.Validate(); err == nil {
		t.Error("oversized attachments accepted")
	}
}

func TestReplyHelpers(t *testing.T) {
	if got := ReplySubject("Re", "Re: Lunch"); got != "Re: Lunch" {
		t.Errorf("got %q", got)
	}
	if got := ReplySubject("Re", "Lunch"); got != "Re: Lunch" {
		t.Errorf("got %q", got)
	}
	if got := ReplySubject("Fwd", "FW: deck"); got != "FW: deck" {
		t.Errorf("got %q", got)
	}
	q := QuoteForReply(pmail.Address{Name: "Omar", Email: "omar@example.com"}, time.Date(2026, 11, 15, 9, 5, 0, 0, time.UTC), "Hi\n> old")
	if !strings.Contains(q, "Omar <omar@example.com> wrote:") || !strings.Contains(q, "> Hi\n>> old") {
		t.Errorf("quote = %q", q)
	}
}

func TestTidyQuote(t *testing.T) {
	in := "Hi team,\n \n \n  Thanks everyone.  \n\n\n\n Bye\n\n"
	if got := Tidy(in); got != "Hi team,\n\nThanks everyone.\n\nBye" {
		t.Errorf("Tidy = %q", got)
	}
	q := QuoteForReply(pmail.Address{Email: "a@example.com"}, time.Now(), in)
	if strings.Contains(q, ">\n>\n") || strings.Contains(q, ">  ") {
		t.Errorf("quote has blank runs or stray spaces: %q", q)
	}
}
