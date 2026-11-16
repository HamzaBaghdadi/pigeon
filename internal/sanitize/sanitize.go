// Package sanitize cleans untrusted email HTML before it reaches the UI.
//
// This is the first of two layers. The second is the sandboxed iframe in the
// frontend, which has no script permission and a CSP that blocks every
// network request unless the user chooses to load remote images.
package sanitize

import (
	"strings"
	"sync"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

var (
	policyOnce sync.Once
	policy     *bluemonday.Policy
)

// safeCSS are the inline style properties email layouts commonly rely on.
var safeCSS = []string{
	"background", "background-color", "border", "border-bottom", "border-collapse",
	"border-color", "border-left", "border-radius", "border-right", "border-spacing",
	"border-style", "border-top", "border-width", "color", "direction", "display",
	"float", "font", "font-family", "font-size", "font-style", "font-weight", "height",
	"letter-spacing", "line-height", "list-style", "list-style-type", "margin",
	"margin-bottom", "margin-left", "margin-right", "margin-top", "max-height",
	"max-width", "min-height", "min-width", "padding", "padding-bottom", "padding-left",
	"padding-right", "padding-top", "table-layout", "text-align", "text-decoration",
	"text-indent", "text-transform", "vertical-align", "white-space", "width",
	"word-break", "word-spacing",
}

func emailPolicy() *bluemonday.Policy {
	policyOnce.Do(func() {
		p := bluemonday.UGCPolicy()
		p.AllowStyles(safeCSS...).Globally()
		p.AllowAttrs("bgcolor", "align", "valign", "width", "height", "border",
			"cellpadding", "cellspacing", "color", "dir").Globally()
		p.AllowElements("center", "font", "span", "div")
		p.AllowURLSchemes("http", "https", "mailto", "cid", "data")
		p.AllowDataURIImages()
		p.RequireNoFollowOnLinks(true)
		p.RequireNoReferrerOnLinks(true)
		policy = p
	})
	return policy
}

// Result is a sanitized email body.
type Result struct {
	HTML string
	// RemoteImages is how many images point at remote servers. They are
	// blocked by the viewer's CSP until the user allows them.
	RemoteImages int
}

// HTML sanitizes an untrusted HTML email body.
func HTML(raw string) Result {
	clean := emailPolicy().Sanitize(raw)
	return Result{HTML: clean, RemoteImages: countRemoteImages(clean)}
}

// countRemoteImages counts <img> tags whose src is an http(s) URL. These are
// the usual vehicle for tracking pixels.
func countRemoteImages(s string) int {
	n := 0
	z := html.NewTokenizer(strings.NewReader(s))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return n
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			if string(name) != "img" || !hasAttr {
				continue
			}
			for {
				key, val, more := z.TagAttr()
				if string(key) == "src" {
					v := strings.ToLower(strings.TrimSpace(string(val)))
					if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "//") {
						n++
					}
				}
				if !more {
					break
				}
			}
		}
	}
}
