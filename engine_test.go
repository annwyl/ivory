package ivory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var runCount atomic.Int64

// blocker just parks until we cancel it
type blocker struct{}

func (blocker) Name() string { return "blocker" }

func (blocker) Run(ctx context.Context, rt *Runtime) error {
	runCount.Add(1)
	rt.Save("", map[string]any{"tick": runCount.Load()})
	<-ctx.Done()
	return nil
}

type memStore struct {
	mu    sync.Mutex
	count int
}

func (m *memStore) Save(key string, record map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.count++
	return nil
}

func (m *memStore) Close() error { return nil }

type panicker struct{}

func (panicker) Name() string { return "panicker" }

func (panicker) Run(ctx context.Context, rt *Runtime) error {
	panic("boom")
}

type describer struct{}

func (describer) Name() string                              { return "describer" }
func (describer) Run(ctx context.Context, _ *Runtime) error { <-ctx.Done(); return nil }
func (describer) Describe() map[string]string               { return map[string]string{"foo": "bar"} }

func init() {
	RegisterCrawler("blocker", func() Crawler { return blocker{} })
	RegisterCrawler("panicker", func() Crawler { return panicker{} })
	RegisterCrawler("describer", func() Crawler { return describer{} })
	RegisterStore("memory", func(json.RawMessage) (Store, error) { return &memStore{}, nil })
}

func TestCrawlerInfo(t *testing.T) {
	e, err := NewEngine(Config{
		Store:    "memory",
		Timeout:  9,
		Crawlers: []string{"describer"},
		Workers:  map[string]int{"describer": 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	info, ok := e.CrawlerInfo("describer")
	if !ok {
		t.Fatal("expected info for a loaded crawler")
	}
	if info.Workers != 2 || info.Timeout != 9 {
		t.Fatalf("unexpected info: %+v", info)
	}
	if info.Params["foo"] != "bar" {
		t.Fatalf("expected Describe params, got %v", info.Params)
	}
	if _, ok := e.CrawlerInfo("ghost"); ok {
		t.Fatal("expected no info for an unknown crawler")
	}
}

func testConfig() Config {
	return Config{Store: "memory", Timeout: 5, Retries: 1, Crawlers: []string{"blocker"}}
}

func TestEngineLifecycle(t *testing.T) {
	e, err := NewEngine(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	if err := e.Start("blocker"); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if err := e.Start("blocker"); err == nil {
		t.Fatal("expected an error when starting an already running crawler")
	}
	if !e.Status()["blocker"] {
		t.Fatal("crawler should be running")
	}

	if err := e.Stop("blocker"); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	waitUntil(t, func() bool { return !e.Status()["blocker"] })
}

func TestEngineReloadMakesFreshInstance(t *testing.T) {
	runCount.Store(0)
	e, err := NewEngine(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	if err := e.Start("blocker"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return runCount.Load() >= 1 })

	if err := e.Reload("blocker"); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	waitUntil(t, func() bool { return runCount.Load() >= 2 })
}

func TestEnginePanicRecovery(t *testing.T) {
	e, err := NewEngine(Config{Store: "memory", Timeout: 5, Crawlers: []string{"panicker"}})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	if err := e.Start("panicker"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return !e.Status()["panicker"] })

	if e.Stats()["panicker"].Errors < 1 {
		t.Fatal("a panic should be counted as an error")
	}
	// the engine survived, so it still answers ig
	if err := e.Start("panicker"); err != nil {
		t.Fatalf("engine should still work after a crawler panic: %v", err)
	}
}

func TestEngineWorkers(t *testing.T) {
	runCount.Store(0)
	e, err := NewEngine(Config{
		Store:    "memory",
		Timeout:  5,
		Crawlers: []string{"blocker"},
		Workers:  map[string]int{"blocker": 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	if err := e.Start("blocker"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return runCount.Load() >= 3 })
	if !e.Status()["blocker"] {
		t.Fatal("crawler should be running")
	}
}

func TestReconfigure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	write := func(s string) {
		if err := os.WriteFile(path, []byte(s), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write(`{"store":"memory","timeout":5,"crawlers":["blocker"],"workers":{"blocker":1}}`)
	config, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	e, err := NewEngine(config)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	// bump blockers workers and add a second crawler
	write(`{"store":"memory","timeout":5,"crawlers":["blocker","describer"],"workers":{"blocker":3}}`)
	if err := e.Reconfigure(path); err != nil {
		t.Fatalf("reconfigure failed: %v", err)
	}

	if info, _ := e.CrawlerInfo("blocker"); info.Workers != 3 {
		t.Fatalf("expected workers updated to 3, got %d", info.Workers)
	}
	if _, ok := e.CrawlerInfo("describer"); !ok {
		t.Fatal("describer should have been added")
	}
	if len(e.Loaded()) != 2 {
		t.Fatalf("expected 2 loaded crawlers, got %d", len(e.Loaded()))
	}
}

func TestEngineUnknownCrawler(t *testing.T) {
	e, err := NewEngine(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	if err := e.Start("ghost"); err == nil {
		t.Fatal("expected an error for an unknown crawler")
	}
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}
