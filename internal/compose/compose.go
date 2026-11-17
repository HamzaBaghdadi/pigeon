// Package compose builds outgoing RFC 5322 messages.
package compose

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net/mail"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	gomail "github.com/emersion/go-message/mail"

	pmail "github.com/HamzaBaghdadi/pigeon/internal/mail"
)

// MaxAttachments is the total attachment size most providers accept.
const MaxAttachments = 25 << 20

// Attachment is a file to send.
type Attachment struct {
	Name        string
	ContentType string
	Data        []byte
}

// Message is an outgoing email.
type Message struct {
	From        pmail.Address
	To, Cc, Bcc []pmail.Address
	Subject     string
	Text        string // plain text as typed; an HTML version is generated
	InReplyTo   string // Message-ID of the message being answered
	References  []string
	Attachments []Attachment
	Date        time.Time
}

// Recipients returns every envelope recipient (To, Cc and Bcc).
func (m Message) Recipients() []string {
	var out []string
	seen := map[string]bool{}
	for _, list := range [][]pmail.Address{m.To, m.Cc, m.Bcc} {
		for _, a := range list {
			k := strings.ToLower(a.Email)
			if !seen[k] {
				seen[k] = true
				out = append(out, a.Email)
			}
		}
	}
	return out
}

// Validate checks the message before sending.
func (m Message) Validate() error {
	if _, err := mail.ParseAddress(m.From.Email); err != nil {
		return fmt.Errorf("invalid sender address %q", m.From.Email)
	}
	if len(m.To)+len(m.Cc)+len(m.Bcc) == 0 {
		return errors.New("add at least one recipient")
	}
	for _, list := range [][]pmail.Address{m.To, m.Cc, m.Bcc} {
		for _, a := range list {
			if _, err := mail.ParseAddress(a.Email); err != nil || strings.ContainsAny(a.Email, "\r\n") {
				return fmt.Errorf("%q isn't a valid email address", a.Email)
			}
		}
	}
	total := 0
	for _, a := range m.Attachments {
		total += len(a.Data)
	}
	if total > MaxAttachments {
		return fmt.Errorf("attachments are %.1f MB; most providers accept up to 25 MB", float64(total)/(1<<20))
	}
	return nil
}

// Build returns the raw message and its Message-ID (without angle brackets).
func Build(m Message) ([]byte, string, error) {
	if err := m.Validate(); err != nil {
		return nil, "", err
	}
	if m.Date.IsZero() {
		m.Date = time.Now()
	}
	var h gomail.Header
	h.SetDate(m.Date)
	h.SetAddressList("From", []*gomail.Address{addr(m.From)})
	if len(m.To) > 0 {
		h.SetAddressList("To", addrs(m.To))
	}
	if len(m.Cc) > 0 {
		h.SetAddressList("Cc", addrs(m.Cc))
	}
	// Bcc recipients get the message through the SMTP envelope only.
	h.SetSubject(oneLine(m.Subject))
	domain := "pigeon.local"
	if at := strings.LastIndex(m.From.Email, "@"); at >= 0 {
		domain = m.From.Email[at+1:]
	}
	if err := h.GenerateMessageIDWithHostname(domain); err != nil {
		return nil, "", err
	}
	msgID, _ := h.MessageID()
	if id := trimID(m.InReplyTo); id != "" {
		h.SetMsgIDList("In-Reply-To", []string{id})
		refs := make([]string, 0, len(m.References)+1)
		for _, r := range m.References {
			if r = trimID(r); r != "" && r != id {
				refs = append(refs, r)
			}
		}
		h.SetMsgIDList("References", append(refs, id))
	}
	h.Set("User-Agent", "Pigeon")

	var buf bytes.Buffer
	mw, err := gomail.CreateWriter(&buf, h)
	if err != nil {
		return nil, "", err
	}
	tw, err := mw.CreateInline()
	if err != nil {
		return nil, "", err
	}
	if err := writePart(tw, "text/plain", normalizeNewlines(m.Text)); err != nil {
		return nil, "", err
	}
	if err := writePart(tw, "text/html", TextToHTML(m.Text)); err != nil {
		return nil, "", err
	}
	if err := tw.Close(); err != nil {
		return nil, "", err
	}
	for _, a := range m.Attachments {
		var ah gomail.AttachmentHeader
		ct := a.ContentType
		if ct == "" {
			ct = mime.TypeByExtension(filepath.Ext(a.Name))
		}
		if ct == "" {
			ct = "application/octet-stream"
		}
		ah.Set("Content-Type", ct)
		ah.SetFilename(safeName(a.Name))
		w, err := mw.CreateAttachment(ah)
		if err != nil {
			return nil, "", err
		}
		if _, err := w.Write(a.Data); err != nil {
			return nil, "", err
		}
		if err := w.Close(); err != nil {
			return nil, "", err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), msgID, nil
}

func writePart(tw *gomail.InlineWriter, contentType, body string) error {
	var ih gomail.InlineHeader
	ih.Set("Content-Type", contentType+"; charset=utf-8")
	ih.Set("Content-Transfer-Encoding", "quoted-printable")
	w, err := tw.CreatePart(ih)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, body); err != nil {
		return err
	}
	return w.Close()
}

var reLink = regexp.MustCompile(`https?://[^\s<>"']+`)

// TextToHTML renders typed text as simple HTML: paragraphs, line breaks,
// clickable links and quoted ("> ") blocks.
func TextToHTML(text string) string {
	var b strings.Builder
	b.WriteString(`<div style="font-family: -apple-system, 'Segoe UI', sans-serif; font-size: 14px; line-height: 1.5">`)
	inQuote := false
	for _, line := range strings.Split(normalizeNewlines(text), "\n") {
		quoted := strings.HasPrefix(line, ">")
		if quoted != inQuote {
			if quoted {
				b.WriteString(`<blockquote style="margin: 0 0 0 .8em; padding-left: 1em; border-left: 3px solid #d4d4da; color: #5c5c66">`)
			} else {
				b.WriteString(`</blockquote>`)
			}
			inQuote = quoted
		}
		if quoted {
			line = strings.TrimPrefix(strings.TrimPrefix(line, ">"), " ")
		}
		esc := html.EscapeString(line)
		esc = reLink.ReplaceAllStringFunc(esc, func(u string) string {
			t := strings.TrimRight(u, ".,;:)")
			return `<a href="` + t + `">` + t + `</a>` + u[len(t):]
		})
		b.WriteString(esc)
		b.WriteString("<br>")
	}
	if inQuote {
		b.WriteString(`</blockquote>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// QuoteForReply prefixes every line with "> " under an attribution line.
func QuoteForReply(from pmail.Address, date time.Time, text string) string {
	who := from.Name
	if who == "" {
		who = from.Email
	} else {
		who += " <" + from.Email + ">"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n\nOn %s, %s wrote:\n", date.Format("Mon, Jan 2, 2006 at 3:04 PM"), who)
	for _, line := range strings.Split(Tidy(text), "\n") {
		switch {
		case line == "":
			b.WriteString(">\n")
		case strings.HasPrefix(line, ">"):
			b.WriteString(">" + line + "\n")
		default:
			b.WriteString("> " + line + "\n")
		}
	}
	return b.String()
}

// Tidy cleans text (often converted from HTML) for quoting: trims each line
// and collapses runs of blank lines into one.
func Tidy(text string) string {
	var out []string
	blank := false
	for _, line := range strings.Split(normalizeNewlines(text), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		blank = false
		out = append(out, line)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// ReplySubject adds "Re: " (or "Fwd: ") once.
func ReplySubject(prefix, subject string) string {
	s := strings.TrimSpace(subject)
	lower := strings.ToLower(s)
	p := strings.ToLower(prefix)
	if strings.HasPrefix(lower, p+":") || (p == "fwd" && strings.HasPrefix(lower, "fw:")) {
		return s
	}
	return prefix + ": " + s
}

func addr(a pmail.Address) *gomail.Address {
	return &gomail.Address{Name: oneLine(a.Name), Address: strings.TrimSpace(a.Email)}
}

func addrs(list []pmail.Address) []*gomail.Address {
	out := make([]*gomail.Address, len(list))
	for i, a := range list {
		out[i] = addr(a)
	}
	return out
}

func trimID(id string) string {
	return strings.Trim(strings.TrimSpace(id), "<>")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.NewReplacer("\r", " ", "\n", " ").Replace(s)), " ")
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

func safeName(n string) string {
	n = filepath.Base(strings.ReplaceAll(n, "\\", "/"))
	n = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`"/\:*?<>|`, r) {
			return '_'
		}
		return r
	}, n)
	if n == "" || n == "." {
		return "attachment"
	}
	return n
}
