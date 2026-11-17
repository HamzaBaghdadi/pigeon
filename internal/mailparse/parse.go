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

// Read limits, so a hostile message can't exhaust memory.
const (
	maxPartSize       = 8 << 20  // text bodies
	maxAttachmentSize = 50 << 20 // a single attachment
	maxInlineKept     = 2 << 20  // inline images kept for display
)

// Body is the parsed content of a message. HTML is untrusted and must be
// sanitized before display (see internal/sanitize).
type Body struct {
	HTML           string
	Text           string
	Snippet        string
	HasAttachments bool // a real attachment (not just an embedded image)
	Parts          []Part
}

// Part is an attachment or an embedded (inline) image.
type Part struct {
	Index       int    // position among non-body parts; stable for a given message
	Filename    string // as sent, unsanitized
	ContentType string
	Size        int64
	ContentID   string // without angle brackets, for cid: references
	Inline      bool   // an image shown inside the HTML body
	Data        []byte // kept only for small inline images
}

// Parse reads a raw message. It is lenient: a malformed message yields
// whatever could be read rather than an error, as long as headers parse.
func Parse(raw []byte) (Body, error) {
	var b Body
	err := walk(raw, func(p Part, data []byte, text, isHTML bool) {
		switch {
		case text && isHTML:
			if b.HTML == "" {
				b.HTML = string(data)
			}
		case text:
			if b.Text == "" {
				b.Text = string(data)
			}
		default:
			if p.Inline && len(data) <= maxInlineKept {
				p.Data = data
			}
			if !p.Inline {
				b.HasAttachments = true
			}
			b.Parts = append(b.Parts, p)
		}
	})
	if err != nil && b.HTML == "" && b.Text == "" && len(b.Parts) == 0 {
		return b, err
	}
	if b.Text == "" && b.HTML != "" {
		b.Text = HTMLToText(b.HTML)
	}
	b.Snippet = Snippet(b.Text, 200)
	return b, nil
}

// PartData returns the decoded bytes of the part with the given index.
func PartData(raw []byte, index int) (Part, []byte, error) {
	var found *Part
	var out []byte
	err := walk(raw, func(p Part, data []byte, text, _ bool) {
		if !text && p.Index == index && found == nil {
			pp := p
			found, out = &pp, data
		}
	})
	if found == nil {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return Part{}, nil, err
	}
	return *found, out, nil
}

// walk visits every leaf part. Body text parts (the first text/plain and
// text/html without a filename) are reported with text=true; everything
// else is numbered as a Part.
func walk(raw []byte, visit func(p Part, data []byte, text, isHTML bool)) error {
	r, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil && r == nil {
		return err
	}
	index := 0
	for {
		p, err := r.NextPart()
		if err == io.EOF {
			return nil
		}
		if err != nil && p == nil {
			return err
		}
		var (
			ct, filename, cid string
			attachment        bool
		)
		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			ct, _, _ = h.ContentType()
			cid = strings.Trim(h.Get("Content-Id"), "<> ")
			if _, params, err := h.ContentDisposition(); err == nil {
				filename = params["filename"]
			}
			if filename == "" {
				_, params, _ := h.ContentType()
				filename = params["name"]
			}
		case *mail.AttachmentHeader:
			ct, _, _ = h.ContentType()
			filename, _ = h.Filename()
			cid = strings.Trim(h.Get("Content-Id"), "<> ")
			attachment = true
		}
		ct = strings.ToLower(ct)
		isText := !attachment && filename == "" && (ct == "text/plain" || ct == "text/html" || ct == "")
		if isText {
			data, _ := io.ReadAll(io.LimitReader(p.Body, maxPartSize))
			visit(Part{}, data, true, ct == "text/html")
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(p.Body, maxAttachmentSize))
		part := Part{
			Index:       index,
			Filename:    filename,
			ContentType: ct,
			Size:        int64(len(data)),
			ContentID:   cid,
			Inline:      cid != "" && strings.HasPrefix(ct, "image/"),
		}
		index++
		visit(part, data, false, false)
	}
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
