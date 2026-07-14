package ivory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type handle struct {
	factory CrawlerFactory
	workers int
	cancel  context.CancelFunc
	active  int
	stats   *crawlerStats
}

type crawlerStats struct {
	saved    atomic.Int64
	errors   atomic.Int64
	lastSave atomic.Int64
	started  atomic.Int64
}

type CrawlerStats struct {
	Running bool
	Saved   int64
	Errors  int64
	LastRun time.Time
	Started time.Time
}

type Engine struct {
	config  Config
	fetcher *Fetcher
	store   Store
	logger  *Logger
	handles map[string]*handle
	wg      sync.WaitGroup
	mutex   sync.Mutex
}

func NewEngine(config Config) (*Engine, error) {
	pool, err := NewPool(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create proxy pool: %v", err)
	}

	store, err := getStore(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create store: %v", err)
	}

	logger, err := NewLogger(config.LogFile, config.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %v", err)
	}

	e := &Engine{
		config:  config,
		fetcher: NewFetcher(config, pool),
		store:   store,
		logger:  logger,
		handles: make(map[string]*handle),
	}

	for _, name := range config.Crawlers {
		factory, err := getFactory(name)
		if err != nil {
			return nil, fmt.Errorf("failed to load crawler %s: %v", name, err)
		}
		workers := config.Workers[name]
		if workers < 1 {
			workers = 1
		}
		e.handles[name] = &handle{factory: factory, workers: workers, stats: &crawlerStats{}}
	}

	if config.StartOnLoad {
		e.StartAll()
	}

	return e, nil
}

func (e *Engine) Start(name string) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.start(name)
}

func (e *Engine) start(name string) error {
	h, ok := e.handles[name]
	if !ok {
		return fmt.Errorf("crawler not loaded: %s", name)
	}
	if h.active > 0 {
		return fmt.Errorf("crawler already running: %s", name)
	}

	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.stats.started.Store(time.Now().UnixNano())

	if h.workers > 1 {
		e.logger.Info(name, fmt.Sprintf("started %d workers", h.workers))
	} else {
		e.logger.Info(name, "started")
	}

	for i := 0; i < h.workers; i++ {
		crawler := h.factory()
		h.active++
		e.wg.Add(1)
		go func(c Crawler) {
			defer e.wg.Done()
			defer func() {
				// dont take the whole engine down
				if r := recover(); r != nil {
					h.stats.errors.Add(1)
					e.logger.Error(name, fmt.Sprintf("recovered from panic: %v", r))
				}
				e.mutex.Lock()
				h.active--
				last := h.active == 0
				e.mutex.Unlock()
				if last {
					e.logger.Info(name, "stopped")
				}
			}()

			rt := &Runtime{name: name, fetcher: e.fetcher, store: e.store, logger: e.logger, stats: h.stats}
			if err := c.Run(ctx, rt); err != nil && ctx.Err() == nil {
				h.stats.errors.Add(1)
				e.logger.Error(name, "error: "+err.Error())
			}
		}(crawler)
	}

	return nil
}

func (e *Engine) Stop(name string) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.stop(name)
}

func (e *Engine) stop(name string) error {
	h, ok := e.handles[name]
	if !ok {
		return fmt.Errorf("crawler not loaded: %s", name)
	}
	if h.active == 0 {
		return fmt.Errorf("crawler not running: %s", name)
	}
	h.cancel()
	return nil
}

func (e *Engine) Reload(name string) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	h, ok := e.handles[name]
	if !ok {
		return fmt.Errorf("crawler not loaded: %s", name)
	}
	if h.active > 0 {
		h.cancel()
	}

	factory, err := getFactory(name)
	if err != nil {
		return fmt.Errorf("failed to reload crawler %s: %v", name, err)
	}
	e.handles[name] = &handle{factory: factory, workers: h.workers, stats: &crawlerStats{}}
	return e.start(name)
}

func (e *Engine) StartAll() {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	for name := range e.handles {
		e.start(name)
	}
}

func (e *Engine) StopAll() {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	for name := range e.handles {
		if e.handles[name].active > 0 {
			e.stop(name)
		}
	}
}

func (e *Engine) Status() map[string]bool {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	status := make(map[string]bool, len(e.handles))
	for name, h := range e.handles {
		status[name] = h.active > 0
	}
	return status
}

func (e *Engine) Stats() map[string]CrawlerStats {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	out := make(map[string]CrawlerStats, len(e.handles))
	for name, h := range e.handles {
		out[name] = statOf(h)
	}
	return out
}

func statOf(h *handle) CrawlerStats {
	running := h.active > 0
	s := CrawlerStats{Running: running, Saved: h.stats.saved.Load(), Errors: h.stats.errors.Load()}
	if ns := h.stats.lastSave.Load(); ns > 0 {
		s.LastRun = time.Unix(0, ns)
	}
	if ns := h.stats.started.Load(); ns > 0 && running {
		s.Started = time.Unix(0, ns)
	}
	return s
}

type CrawlerInfo struct {
	Name          string
	Workers       int
	Timeout       int
	Retries       int
	RateLimit     int
	MaxConcurrent int
	Params        map[string]string
	Stats         CrawlerStats
}

func (e *Engine) CrawlerInfo(name string) (CrawlerInfo, bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	h, ok := e.handles[name]
	if !ok {
		return CrawlerInfo{}, false
	}
	info := CrawlerInfo{
		Name:          name,
		Workers:       h.workers,
		Timeout:       e.config.Timeout,
		Retries:       e.config.Retries,
		RateLimit:     e.config.RateLimit,
		MaxConcurrent: e.config.MaxConcurrent,
		Stats:         statOf(h),
	}
	if d, ok := h.factory().(Describable); ok {
		info.Params = d.Describe()
	}
	return info, true
}

func (e *Engine) CrawlerLogs(crawler string, n int) []string {
	return e.logger.RecentFor(crawler, n)
}

func (e *Engine) ProxyStats() []ProxyStat {
	return e.fetcher.ProxyStats()
}

func (e *Engine) Strategy() string {
	if e.config.ProxyStrategy == "" {
		return "round_robin"
	}
	return e.config.ProxyStrategy
}

func (e *Engine) RefreshInterval() time.Duration {
	if e.config.RefreshInterval <= 0 {
		return time.Second
	}
	return time.Duration(e.config.RefreshInterval) * time.Millisecond
}

func (e *Engine) Loaded() []string {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	names := make([]string, 0, len(e.handles))
	for name := range e.handles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (e *Engine) SetConsole(on bool) {
	e.logger.SetConsole(on)
}

func (e *Engine) Logs(n int) []string {
	return e.logger.Recent(n)
}

// reconfigure reloads conf from disk
func (e *Engine) Reconfigure(path string) error {
	config, err := LoadConfig(path)
	if err != nil {
		e.logger.Error("engine", "config reload failed: "+err.Error())
		return err
	}
	pool, err := NewPool(config)
	if err != nil {
		e.logger.Error("engine", "config reload failed: "+err.Error())
		return err
	}

	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.config = config
	e.fetcher = NewFetcher(config, pool)
	e.logger.SetLevel(config.LogLevel)

	seen := make(map[string]bool)
	for _, name := range config.Crawlers {
		seen[name] = true
		workers := config.Workers[name]
		if workers < 1 {
			workers = 1
		}
		if h, ok := e.handles[name]; ok {
			h.workers = workers
			continue
		}
		factory, err := getFactory(name)
		if err != nil {
			e.logger.Error("engine", "config reload: "+err.Error())
			continue
		}
		e.handles[name] = &handle{factory: factory, workers: workers, stats: &crawlerStats{}}
	}
	for name, h := range e.handles {
		if !seen[name] {
			if h.active > 0 {
				h.cancel()
			}
			delete(e.handles, name)
		}
	}

	e.logger.Info("engine", "config reloaded")
	return nil
}

func (e *Engine) Query(term string, limit int) ([]map[string]any, error) {
	q, ok := e.store.(Queryable)
	if !ok {
		return nil, fmt.Errorf("store %q does not support querying", e.config.Store)
	}
	return q.Query(term, limit)
}

func (e *Engine) Close() error {
	e.StopAll()
	e.wg.Wait()
	if err := e.store.Close(); err != nil {
		return err
	}
	return e.logger.Close()
}
