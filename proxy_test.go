package ivory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPoolRoundRobin(t *testing.T) {
	pool, err := NewPool(Config{Proxies: []string{"http://a:1", "http://b:2"}})
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]int{}
	for i := 0; i < 4; i++ {
		seen[pool.Next().url.Host]++
	}
	if seen["a:1"] == 0 || seen["b:2"] == 0 {
		t.Fatalf("expected both proxies to be used, got %v", seen)
	}
}

func TestPoolEmpty(t *testing.T) {
	pool, err := NewPool(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if pool.Next() != nil {
		t.Fatal("expected nil proxy when none configured")
	}
}

func TestPoolBadProxy(t *testing.T) {
	if _, err := NewPool(Config{Proxies: []string{"://broken"}}); err == nil {
		t.Fatal("expected an error for an unparseable proxy")
	}
}

func TestPoolDisablesAndRecovers(t *testing.T) {
	pool, err := NewPool(Config{Proxies: []string{"http://a:1"}})
	if err != nil {
		t.Fatal(err)
	}
	pool.maxFails = 2
	pool.cooldown = 40 * time.Millisecond

	p := pool.Next()
	pool.Report(p, false, 0)
	pool.Report(p, false, 0)

	if pool.Snapshot()[0].Live {
		t.Fatal("proxy should be disabled after hitting max fails")
	}

	time.Sleep(60 * time.Millisecond)
	if !pool.Snapshot()[0].Live {
		t.Fatal("proxy should recover after the cooldown")
	}
}

func TestPoolWindowRate(t *testing.T) {
	pool, err := NewPool(Config{Proxies: []string{"http://a:1"}})
	if err != nil {
		t.Fatal(err)
	}
	pool.windowSize = 4

	p := pool.Next()
	pool.Report(p, true, 0)
	pool.Report(p, true, 0)
	pool.Report(p, false, 0)
	pool.Report(p, true, 0)

	if rate := pool.Snapshot()[0].Rate; rate < 0.74 || rate > 0.76 {
		t.Fatalf("expected rate ~0.75, got %v", rate)
	}
}

func TestPoolLoadsFromDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("# a comment\nhttp://a:1\n\nhttp://b:2\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.json"), []byte(`["http://c:3","http://a:1"]`), 0644)

	pool, err := NewPool(Config{Proxies: []string{"http://d:4"}, ProxyDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	// d:4 inline + a:1, b:2 from txt + c:3 from json with the duplicate a:1 dropped
	if pool.Size() != 4 {
		t.Fatalf("expected 4 proxies, got %d", pool.Size())
	}
}
