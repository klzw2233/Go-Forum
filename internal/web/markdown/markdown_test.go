package markdown

import (
	"strings"
	"testing"
)

func TestRenderHTTPSImage(t *testing.T) {
	html := Render("![x](https://example.com/a.png)")
	if !strings.Contains(html, `<img`) {
		t.Fatalf("expected img, got %q", html)
	}
	if !strings.Contains(html, `src="https://example.com/a.png"`) {
		t.Fatalf("expected https src, got %q", html)
	}
}

func TestRenderHTTPImage(t *testing.T) {
	html := Render("![x](http://example.com/a.png)")
	if !strings.Contains(html, `src="http://example.com/a.png"`) {
		t.Fatalf("expected http src, got %q", html)
	}
}

func TestRejectJavascriptImage(t *testing.T) {
	html := Render("![x](javascript:alert(1))")
	low := strings.ToLower(html)
	if strings.Contains(low, "javascript:") {
		t.Fatalf("javascript leaked: %q", html)
	}
	if strings.Contains(low, "<script") {
		t.Fatalf("script leaked: %q", html)
	}
}

func TestRejectDataImage(t *testing.T) {
	html := Render("![x](data:image/png;base64,aaaa)")
	low := strings.ToLower(html)
	if strings.Contains(low, "data:") {
		t.Fatalf("data uri leaked: %q", html)
	}
}

func TestRejectScript(t *testing.T) {
	html := Render("<script>alert(1)</script>")
	if strings.Contains(strings.ToLower(html), "<script") {
		t.Fatalf("script leaked: %q", html)
	}
}
