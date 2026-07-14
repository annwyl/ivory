package crawlers

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/annwyl/ivory"
)

type RSS struct {
	feed     string
	interval time.Duration
}

func init() {
	err := ivory.RegisterCrawler("rss", func() ivory.Crawler {
		return &RSS{feed: "https://feeds.bbci.co.uk/news/rss.xml", interval: 15 * time.Minute}
	})
	if err != nil {
		panic(err)
	}
}

func (r *RSS) Name() string { return "rss" }

func (r *RSS) Describe() map[string]string {
	return map[string]string{"feed": r.feed, "interval": r.interval.String()}
}

func (r *RSS) Run(ctx context.Context, rt *ivory.Runtime) error {
	for {
		if err := r.crawl(ctx, rt); err != nil {
			rt.Errorf("crawl failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(r.interval):
		}
	}
}

type rssFeed struct {
	Items []rssItem `xml:"channel>item"`
}

type rssItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	GUID    string `xml:"guid"`
	PubDate string `xml:"pubDate"`
}

func (r *RSS) crawl(ctx context.Context, rt *ivory.Runtime) error {
	resp, err := rt.Get(ctx, r.feed)
	if err != nil {
		return err
	}

	var feed rssFeed
	if err := xml.Unmarshal(resp.Body, &feed); err != nil {
		return fmt.Errorf("failed to parse feed: %v", err)
	}

	for _, item := range feed.Items {
		key := item.GUID
		if key == "" {
			key = item.Link
		}
		rt.Save(key, map[string]any{
			"title":      item.Title,
			"link":       item.Link,
			"published":  item.PubDate,
			"crawled_at": time.Now().Format(time.RFC3339),
		})
	}

	rt.Log(fmt.Sprintf("saved %d items", len(feed.Items)))
	return nil
}
