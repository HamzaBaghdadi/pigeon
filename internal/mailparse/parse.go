// Package mailparse extracts displayable content from raw RFC 5322 messages.
package mailparse

import (
	"bytes"
	"html"
	"io"
	"regexp"
	"strings"

	_ "github.com/emersion/go-message/charset" // decode non-UTF-8 charsets (ISO-8859-*, Windows-125x, ...)
	"github.com/emersion/go-message/mail"
)

// maxPartSize bounds how much of any single part is read, so a hostile
// message can't exhaust memory.
const maxPartSize = 8 << 20

// Body is the parsed content of a message. HTML is untrusted and must be
// sanitized before display (see internal/sanitize).
type Body struct {
	HTML           string
	Text           string
	Snippet        string
	HasAttachments bool
}

// Parse reads a raw message. It is lenient: a malformed message yields
// whatever could be read rather than an error, as long as headers parse.
func Parse(raw []byte) (Body, error) {
	var b Body
	r, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil && r == nil {
		return b, err
	}
	for {
		p, err := r.NextPart()
		if err == io.EOF || (err != nil && p == nil) {
			break
		}
		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			ct, _, _ := h.ContentType()
			data, _ := io.ReadAll(io.LimitReader(p.Body, maxPartSize))
			switch ct {
			case "text/html":
				if b.HTML == "" {
					b.HTML = string(data)
				}
			case "text/plain", "":
				if b.Text == "" {
					b.Text = string(data)
				}
			default:
				if strings.HasPrefix(ct, "image/") || strings.HasPrefix(ct, "application/") {
					b.HasAttachments = true
				}
			}
		case *mail.AttachmentHeader:
			b.HasAttachments = true
		}
	}
	if b.Text == "" && b.HTML != "" {
		b.Text = HTMLToText(b.HTML)
	}
	b.Snippet = Snippet(b.Text, 200)
	return b, nil
}

var (
	reBlock   = regexp.MustCompile(`(?is)<(script|style|head|title)[^>]*>.*?</(script|style|head|title)>`)
	reBreak   = regexp.MustCompile(`(?i)<(br|/p|/div|/tr|/li|/h[1-6])[^>]*>`)
	reTag     = regexp.MustCompile(`(?s)<[^>]+>`)
	reSpace   = regexp.MustCompile(`[ \t\r\f\v\x{00a0}]+`)
	reNewline = regexp.MustCompile(`\n{3,}`)
)

// HTMLToText is a rough conversion used for search indexing and snippets.
func HTMLToText(s string) string {
	s = reBlock.ReplaceAllString(s, " ")
	s = reBreak.ReplaceAllString(s, "\n")
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = reSpace.ReplaceAllString(s, " ")
	s = reNewline.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

var reQuoteIntro = regexp.MustCompile(`(?m)^On .{5,200} wrote:\s*$`)

// Snippet is a one-line preview: quoted replies dropped, whitespace collapsed.
func Snippet(text string, max int) string {
	if loc := reQuoteIntro.FindStringIndex(text); loc != nil {
		text = text[:loc[0]]
	}
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, ">") {
			continue
		}
		lines = append(lines, l)
	}
	s := strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
	if r := []rune(s); len(r) > max {
		s = strings.TrimSpace(string(r[:max])) + "…"
	}
	return s
}
