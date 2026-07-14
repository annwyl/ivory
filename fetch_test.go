package ivory

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetcherEatsA500ThenSucceeds(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	pool, _ := NewPool(Config{})
	f := NewFetcher(Config{Timeout: 5, Retries: 3}, pool)

	resp, err := f.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if resp.StatusCode != 200 || string(resp.Body) != "ok" {
		t.Fatalf("unexpected response: %d %q", resp.StatusCode, resp.Body)
	}
	if hits.Load() < 2 {
		t.Fatalf("expected at least one retry, got %d hits", hits.Load())
	}
}

func TestFetcherSetsUserAgent(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("User-Agent")
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	pool, _ := NewPool(Config{})
	f := NewFetcher(Config{Timeout: 5, UserAgents: []string{"probe/1"}}, pool)
	if _, err := f.Get(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	if seen != "probe/1" {
		t.Fatalf("user agent not set, got %q", seen)
	}
}

func TestFetcherMaxConcurrent(t *testing.T) {
	var current, peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := current.Add(1)
		for {
			p := peak.Load()
			if c <= p || peak.CompareAndSwap(p, c) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		current.Add(-1)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	pool, _ := NewPool(Config{})
	f := NewFetcher(Config{Timeout: 5, MaxConcurrent: 2}, pool)

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.Get(context.Background(), srv.URL)
		}()
	}
	wg.Wait()

	if peak.Load() > 2 {
		t.Fatalf("more than 2 requests ran at once, peak was %d", peak.Load())
	}
}

func TestFetcherRetriesOn429(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	pool, _ := NewPool(Config{})
	f := NewFetcher(Config{Timeout: 5, Retries: 3}, pool)

	resp, err := f.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 after a 429, got %d", resp.StatusCode)
	}
	if hits.Load() < 2 {
		t.Fatalf("expected a retry after 429, got %d hits", hits.Load())
	}
}

func TestRetryAfterParsing(t *testing.T) {
	resp := &Response{Header: http.Header{"Retry-After": []string{"5"}}}
	if got := retryAfter(resp); got != 5*time.Second {
		t.Fatalf("expected 5s, got %v", got)
	}
	if got := retryAfter(&Response{Header: http.Header{}}); got != 0 {
		t.Fatalf("expected 0 when absent, got %v", got)
	}
}

func TestFetcherRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	pool, _ := NewPool(Config{})
	f := NewFetcher(Config{Timeout: 5, RateLimit: 150}, pool)

	start := time.Now()
	f.Get(context.Background(), srv.URL)
	f.Get(context.Background(), srv.URL)
	elapsed := time.Since(start)

	if elapsed < 150*time.Millisecond {
		t.Fatalf("second request did not respect the rate limit, took %v", elapsed)
	}
}
