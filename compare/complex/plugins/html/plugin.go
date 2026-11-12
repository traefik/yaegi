package htmlplug

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

// state renders Markdown with goldmark, sanitizes it with bluemonday and
// queries the HTML with goquery (CSS selectors on x/net/html).
type state struct {
	policy *bluemonday.Policy
	md     goldmark.Markdown
}

const page = `
# Title for PATH

Some *text* with a [link](https://example.org/?q=1) and <script>alert(1)</script>.

- one
- two <img src=x onerror=alert(1)>
- three

| a | b |
|---|---|
| 1 | 2 |
`

func setup(_ context.Context) (*state, error) {
	p := bluemonday.UGCPolicy()
	p.RequireNoFollowOnLinks(true)
	return &state{policy: p, md: goldmark.New()}, nil
}

func (s *state) process(req *http.Request) (string, error) {
	var raw strings.Builder
	if err := s.md.Convert([]byte(strings.ReplaceAll(page, "PATH", req.URL.Path)), &raw); err != nil {
		return "", err
	}
	clean := s.policy.Sanitize(raw.String())
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(clean))
	if err != nil {
		return "", err
	}
	var items []string
	doc.Find("ul > li").Each(func(_ int, sel *goquery.Selection) {
		items = append(items, strings.TrimSpace(sel.Text()))
	})
	href, _ := doc.Find("a").First().Attr("href")
	rel, _ := doc.Find("a").First().Attr("rel")
	return fmt.Sprintf("h1=%q items=%q href=%s rel=%s script=%t onerror=%t len=%d/%d",
		doc.Find("h1").Text(), items, href, rel, strings.Contains(clean, "<script"), strings.Contains(clean, "onerror"),
		raw.Len(), len(clean)), nil
}
