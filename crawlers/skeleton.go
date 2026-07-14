package crawlers

import (
	"context"
	"time"

	"github.com/annwyl/ivory"
)

//starting point, copy this file, rename the type, change the name you register and put your logic in crawl
type Skeleton struct {
	target   string
	interval time.Duration
}

func init() {
	err := ivory.RegisterCrawler("skeleton", func() ivory.Crawler {
		return &Skeleton{target: "https://example.com", interval: time.Hour}
	})
	if err != nil {
		panic(err)
	}
}

func (s *Skeleton) Name() string { return "skeleton" }

// optional, whatever you return shows up in the detailed view
func (s *Skeleton) Describe() map[string]string {
	return map[string]string{"target": s.target, "interval": s.interval.String()}
}

func (s *Skeleton) Run(ctx context.Context, rt *ivory.Runtime) error {
	for {
		if err := s.crawl(ctx, rt); err != nil {
			rt.Errorf("crawl failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(s.interval):
		}
	}
}

func (s *Skeleton) crawl(ctx context.Context, rt *ivory.Runtime) error {
	resp, err := rt.Get(ctx, s.target)
	if err != nil {
		return err
	}
	// parse resp.Body and pull out what you care about, then save it with a stable key for recrawls
	rt.Save(s.target, map[string]any{
		"url":        s.target,
		"bytes":      len(resp.Body),
		"crawled_at": time.Now().Format(time.RFC3339),
	})
	rt.Log("saved 1 record")
	return nil
}
