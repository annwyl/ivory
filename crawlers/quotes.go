package crawlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/annwyl/ivory"
	"golang.org/x/net/html"
)

type Quotes struct {
	start    string
	maxPages int
	interval time.Duration
}

func init() {
	err := ivory.RegisterCrawler("quotes", func() ivory.Crawler {
		return &Quotes{start: "https://quotes.toscrape.com/", maxPages: 5, interval: 30 * time.Minute}
	})
	if err != nil {
		panic(err)
	}
}

func (q *Quotes) Name() string { return "quotes" }

func (q *Quotes) Describe() map[string]string {
	return map[string]string{
		"start":     q.start,
		"max pages": fmt.Sprintf("%d", q.maxPages),
		"interval":  q.interval.String(),
	}
}

func (q *Quotes) Run(ctx context.Context, rt *ivory.Runtime) error {
	for {
		if err := q.crawl(ctx, rt); err != nil {
			rt.Errorf("crawl failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(q.interval):
		}
	}
}

func (q *Quotes) crawl(ctx context.Context, rt *ivory.Runtime) error {
	next := q.start
	saved := 0
	for page := 1; page <= q.maxPages && next != ""; page++ {
		if ctx.Err() != nil {
			return nil
		}

		resp, err := rt.Get(ctx, next)
		if err != nil {
			return err
		}
		doc, err := html.Parse(bytes.NewReader(resp.Body))
		if err != nil {
			return fmt.Errorf("failed to parse html: %v", err)
		}

		for _, node := range findByClass(doc, "div", "quote") {
			text := nodeText(firstByClass(node, "span", "text"))
			if text == "" {
				continue
			}
			key := fmt.Sprintf("%x", sha256.Sum256([]byte(text)))[:16]
			rt.Save(key, map[string]any{
				"text":       text,
				"author":     nodeText(firstByClass(node, "small", "author")),
				"url":        next,
				"page":       page,
				"crawled_at": time.Now().Format(time.RFC3339),
			})
			saved++
		}

		next = nextPage(next, doc)
	}

	rt.Log(fmt.Sprintf("saved %d quotes", saved))
	return nil
}

func nextPage(current string, doc *html.Node) string {
	li := firstByClass(doc, "li", "next")
	if li == nil {
		return ""
	}
	for _, a := range elements(li, "a") {
		if href := attr(a, "href"); href != "" {
			return resolve(current, href)
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func findByClass(root *html.Node, tag, class string) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tag && hasClass(n, class) {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}

func firstByClass(root *html.Node, tag, class string) *html.Node {
	if nodes := findByClass(root, tag, class); len(nodes) > 0 {
		return nodes[0]
	}
	return nil
}

func elements(root *html.Node, tag string) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tag {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}

func nodeText(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(b.String())
}

func resolve(base, href string) string {
	b, err := url.Parse(base)
	if err != nil {
		return href
	}
	r, err := url.Parse(href)
	if err != nil {
		return href
	}
	return b.ResolveReference(r).String()
}
