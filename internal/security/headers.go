// Package security holds hardening applied to the embedded web UI.
package security

import "net/http"

// contentSecurityPolicy locks the app shell down to its own bundled assets.
// Email bodies are never rendered in this document: they go into a sandboxed
// iframe that carries its own, stricter policy (see frontend MessageBody).
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline'; " + // Next.js static export inlines its bootstrap scripts
	"style-src 'self' 'unsafe-inline'; " +
	// Remote images are allowed here only because the email iframe inherits this
	// policy; the iframe's own CSP blocks them until the user opts in per message.
	"img-src 'self' data: blob: https: http:; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"frame-src 'self' about: data:; " +
	"object-src 'none'; base-uri 'none'; form-action 'none'"

// Headers adds security headers to every response from the asset server.
// It is a no-op in dev builds so the Next.js dev server (eval, HMR) keeps working.
func Headers(next http.Handler) http.Handler {
	if !production {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
