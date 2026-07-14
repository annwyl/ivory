package ivory

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const defaultMaxBody = 10 << 20 // 10 MiB

type Response struct {
	URL        string
	StatusCode int
	Header     http.Header
	Body       []byte
}

type Fetcher struct {
	pool       *Pool
	timeout    time.Duration
	userAgents []string
	retries    int
	rateLimit  time.Duration
	maxBody    int64
	sem        chan struct{}

	direct   *http.Client
	clients  map[string]*http.Client
	clientMu sync.Mutex

	nextAllowed map[string]time.Time
	mutex       sync.Mutex
}

func NewFetcher(config Config, pool *Pool) *Fetcher {
	timeout := time.Duration(config.Timeout) * time.Second
	maxBody := int64(config.MaxBodyBytes)
	if maxBody <= 0 {
		maxBody = defaultMaxBody
	}
	f := &Fetcher{
		pool:        pool,
		timeout:     timeout,
		userAgents:  config.UserAgents,
		retries:     config.Retries,
		rateLimit:   time.Duration(config.RateLimit) * time.Millisecond,
		maxBody:     maxBody,
		direct:      &http.Client{Timeout: timeout},
		clients:     make(map[string]*http.Client),
		nextAllowed: make(map[string]time.Time),
	}
	if config.MaxConcurrent > 0 {
		f.sem = make(chan struct{}, config.MaxConcurrent)
	}
	return f
}

func (f *Fetcher) ProxyStats() []ProxyStat {
	return f.pool.Snapshot()
}

func (f *Fetcher) Get(ctx context.Context, target string) (*Response, error) {
	if err := f.wait(ctx, target); err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt <= f.retries; attempt++ {
		proxy := f.pool.Next()
		if !f.acquire(ctx) {
			return nil, ctx.Err()
		}
		start := time.Now()
		resp, err := f.do(ctx, f.clientFor(proxy), target)
		f.release()
		f.pool.Report(proxy, err == nil, time.Since(start))

		if err != nil {
			lastErr = err
			if !f.backoff(ctx, attempt, 0) {
				return nil, ctx.Err()
			}
			continue
		}

		switch {
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable:
			lastErr = fmt.Errorf("rate limited with status %d", resp.StatusCode)
			if !f.backoff(ctx, attempt, retryAfter(resp)) {
				return nil, ctx.Err()
			}
		case resp.StatusCode >= 500:
			lastErr = fmt.Errorf("server responded with status %d", resp.StatusCode)
			if !f.backoff(ctx, attempt, 0) {
				return nil, ctx.Err()
			}
		default:
			return resp, nil
		}
	}

	return nil, fmt.Errorf("failed to fetch %s after %d attempts: %v", target, f.retries+1, lastErr)
}

func (f *Fetcher) do(ctx context.Context, client *http.Client, target string) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("User-Agent", f.userAgent())

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// read one byte past the limit so we can tell the exact limit and not be over
	body, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %v", err)
	}
	if int64(len(body)) > f.maxBody {
		return nil, fmt.Errorf("response body larger than %d bytes", f.maxBody)
	}

	return &Response{
		URL:        target,
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       body,
	}, nil
}

// clientFor reuse http.Client per proxy to stay pooled
func (f *Fetcher) clientFor(proxy *Proxy) *http.Client {
	if proxy == nil {
		return f.direct
	}
	f.clientMu.Lock()
	defer f.clientMu.Unlock()
	key := proxy.url.String()
	if c, ok := f.clients[key]; ok {
		return c
	}
	c := &http.Client{
		Timeout:   f.timeout,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxy.url)},
	}
	f.clients[key] = c
	return c
}

// blocks, but bails if crawler is stopped
func (f *Fetcher) acquire(ctx context.Context) bool {
	if f.sem == nil {
		return true
	}
	select {
	case f.sem <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (f *Fetcher) release() {
	if f.sem != nil {
		<-f.sem
	}
}

func (f *Fetcher) wait(ctx context.Context, target string) error {
	if f.rateLimit <= 0 {
		return nil
	}
	host := hostOf(target)

	f.mutex.Lock()
	now := time.Now()
	slot := f.nextAllowed[host]
	if slot.Before(now) {
		slot = now
	}
	f.nextAllowed[host] = slot.Add(f.rateLimit)
	f.prune(now)
	f.mutex.Unlock()

	delay := time.Until(slot)
	if delay <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

// prob better to track next expiration and maintain min heap ordered... keeping it for now as its simple. caller holds the mutex
func (f *Fetcher) prune(now time.Time) {
	if len(f.nextAllowed) < 4096 {
		return
	}
	for host, slot := range f.nextAllowed {
		if slot.Before(now) {
			delete(f.nextAllowed, host)
		}
	}
}

func (f *Fetcher) backoff(ctx context.Context, attempt int, hint time.Duration) bool {
	if attempt >= f.retries {
		return true
	}
	delay := hint
	if delay <= 0 {
		delay = time.Duration(attempt+1) * time.Second
	}
	if delay > 2*time.Minute {
		delay = 2 * time.Minute
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(delay):
		return true
	}
}

func (f *Fetcher) userAgent() string {
	if len(f.userAgents) == 0 {
		return "ivory/0.1"
	}
	return f.userAgents[rand.Intn(len(f.userAgents))]
}

func hostOf(target string) string {
	u, err := url.Parse(target)
	if err != nil {
		return target
	}
	return u.Host
}

func retryAfter(resp *Response) time.Duration {
	value := resp.Header.Get("Retry-After")
	if value == "" {
		return 0
	}
	if secs, err := strconv.Atoi(value); err == nil {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(value); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
