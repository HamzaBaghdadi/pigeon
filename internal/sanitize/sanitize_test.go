package sanitize

import (
	"strings"
	"testing"
)

func TestStripsScriptsAndHandlers(t *testing.T) {
	in := `<p onclick="steal()">Hi</p><script>alert(1)</script><a href="javascript:alert(1)">x</a><iframe src="https://evil"></iframe>`
	out := HTML(in).HTML
	for _, bad := range []string{"<script", "onclick", "javascript:", "<iframe"} {
		if strings.Contains(strings.ToLower(out), bad) {
			t.Errorf("sanitized output still contains %q: %s", bad, out)
		}
	}
}

func TestKeepsLayoutStyles(t *testing.T) {
	out := HTML(`<td style="color: #333; padding: 8px; position: fixed">x</td>`).HTML
	if !strings.Contains(out, "color") || !strings.Contains(out, "padding") {
		t.Errorf("expected safe styles to survive: %s", out)
	}
	if strings.Contains(out, "position") {
		t.Errorf("expected position to be stripped: %s", out)
	}
}

func TestCountsRemoteImages(t *testing.T) {
	r := HTML(`<img src="https://t.example/p.gif"><img src="data:image/png;base64,AAAA"><img src="//cdn.example/a.png">`)
	if r.RemoteImages != 2 {
		t.Errorf("RemoteImages = %d, want 2", r.RemoteImages)
	}
}
