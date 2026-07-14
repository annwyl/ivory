package crawlers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/annwyl/ivory"
)

type HackerNews struct {
	interval time.Duration
	limit    int
}

func init() {
	err := ivory.RegisterCrawler("hackernews", func() ivory.Crawler {
		return &HackerNews{
			interval: 5 * time.Minute,
			limit:    30,
		}
	})
	if err != nil {
		panic(err)
	}
}

func (h *HackerNews) Name() string {
	return "hackernews"
}

func (h *HackerNews) Describe() map[string]string {
	return map[string]string{
		"source":   "hacker news firebase api",
		"interval": h.interval.String(),
		"limit":    fmt.Sprintf("%d stories", h.limit),
	}
}

func (h *HackerNews) Run(ctx context.Context, rt *ivory.Runtime) error {
	for {
		if err := h.crawl(ctx, rt); err != nil {
			rt.Log("crawl failed: " + err.Error())
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(h.interval):
		}
	}
}

func (h *HackerNews) crawl(ctx context.Context, rt *ivory.Runtime) error {
	resp, err := rt.Get(ctx, "https://hacker-news.firebaseio.com/v0/topstories.json")
	if err != nil {
		return err
	}

	var ids []int
	if err := json.Unmarshal(resp.Body, &ids); err != nil {
		return fmt.Errorf("failed to decode story ids: %v", err)
	}

	if len(ids) > h.limit {
		ids = ids[:h.limit]
	}

	for _, id := range ids {
		if ctx.Err() != nil {
			return nil
		}

		item, err := rt.Get(ctx, fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%d.json", id))
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			rt.Errorf("failed to fetch item %d: %v", id, err)
			continue
		}

		var story map[string]any
		if err := json.Unmarshal(item.Body, &story); err != nil {
			rt.Errorf("failed to decode item %d: %v", id, err)
			continue
		}

		story["crawled_at"] = time.Now().Format(time.RFC3339)
		// the story id is a stable key so recrawls update rather than duplicate
		if err := rt.Save(fmt.Sprintf("%d", id), story); err != nil {
			rt.Errorf("failed to save item %d: %v", id, err)
		}
	}

	rt.Log(fmt.Sprintf("saved %d stories", len(ids)))
	return nil
}
