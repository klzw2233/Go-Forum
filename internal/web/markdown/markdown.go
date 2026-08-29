package markdown

import (
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

var (
	md = goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
	policy = newPolicy()
)

func newPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowImages()
	p.RequireNoFollowOnLinks(false)
	p.AllowAttrs("src").Matching(regexp.MustCompile(`(?i)^https?://`)).OnElements("img")
	p.AllowAttrs("alt", "title").OnElements("img")
	p.AllowAttrs("href").Matching(regexp.MustCompile(`(?i)^https?://`)).OnElements("a")
	return p
}

func Render(src string) string {
	var buf strings.Builder
	if err := md.Convert([]byte(src), &buf); err != nil {
		return ""
	}
	out := policy.Sanitize(buf.String())
	return out
}
