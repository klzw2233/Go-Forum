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
	p.AllowAttrs("href").Matching(regexp.MustCompile(`(?i)^(https?://|/u/[A-Za-z][A-Za-z0-9_]{0,31})$`)).OnElements("a")
	return p
}

var mentionRe = regexp.MustCompile(`(^|[\s>])@([A-Za-z][A-Za-z0-9_]{0,31})\b`)

func linkMentions(html string) string {
	return mentionRe.ReplaceAllString(html, `$1<a href="/u/$2">@$2</a>`)
}

func Render(src string) string {
	var buf strings.Builder
	if err := md.Convert([]byte(src), &buf); err != nil {
		return ""
	}
	return policy.Sanitize(linkMentions(buf.String()))
}
