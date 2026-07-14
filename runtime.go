package ivory

import (
	"context"
	"fmt"
	"time"
)

type Runtime struct {
	name    string
	fetcher *Fetcher
	store   Store
	logger  *Logger
	stats   *crawlerStats
}

func (rt *Runtime) Get(ctx context.Context, url string) (*Response, error) {
	resp, err := rt.fetcher.Get(ctx, url)
	// a cancelled context is us stopping the crawler not a real error (hopefully, dont let a crawler wrap its own context with its own timeout)
	if err != nil && ctx.Err() == nil && rt.stats != nil {
		rt.stats.errors.Add(1)
	}
	return resp, err
}

func (rt *Runtime) Save(key string, record map[string]any) error {
	if err := rt.store.Save(key, record); err != nil {
		if rt.stats != nil {
			rt.stats.errors.Add(1)
		}
		return err
	}
	if rt.stats != nil {
		rt.stats.saved.Add(1)
		rt.stats.lastSave.Store(time.Now().UnixNano())
	}
	return nil
}

func (rt *Runtime) Log(message string) {
	rt.logger.Info(rt.name, message)
}

func (rt *Runtime) Errorf(format string, args ...any) {
	rt.logger.Error(rt.name, fmt.Sprintf(format, args...))
}
