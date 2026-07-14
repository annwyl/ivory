package ivory

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Proxy struct {
	url              *url.URL
	ok               int
	fail             int
	consecutiveFails int
	disabledUntil    time.Time
	lastUsed         time.Time
	window           []bool
	latencyTotal     time.Duration
	latencyCount     int
}

type ProxyStat struct {
	Host       string
	Live       bool
	OK         int
	Fail       int
	Rate       float64
	LastUsed   time.Time
	AvgLatency time.Duration
}

type Pool struct {
	proxies    []*Proxy
	index      int
	strategy   string
	maxFails   int
	cooldown   time.Duration
	windowSize int
	mutex      sync.Mutex
}

func NewPool(config Config) (*Pool, error) {
	raw, err := gatherProxies(config)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var proxies []*Proxy
	for _, p := range raw {
		if seen[p] {
			continue
		}
		seen[p] = true
		u, err := url.Parse(p)
		if err != nil {
			return nil, fmt.Errorf("failed to parse proxy %s: %v", p, err)
		}
		proxies = append(proxies, &Proxy{url: u})
	}

	pool := &Pool{
		proxies:    proxies,
		strategy:   config.ProxyStrategy,
		maxFails:   config.ProxyMaxFails,
		cooldown:   time.Duration(config.ProxyCooldown) * time.Second,
		windowSize: config.ProxyWindow,
	}
	if pool.strategy == "" {
		pool.strategy = "round_robin"
	}
	if pool.maxFails <= 0 {
		pool.maxFails = 3
	}
	if pool.cooldown <= 0 {
		pool.cooldown = 30 * time.Second
	}
	if pool.windowSize <= 0 {
		pool.windowSize = 20
	}
	return pool, nil
}

func (p *Pool) Next() *Proxy {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if len(p.proxies) == 0 {
		return nil
	}

	now := time.Now()
	live := make([]*Proxy, 0, len(p.proxies))
	for _, pr := range p.proxies {
		if pr.disabledUntil.Before(now) {
			live = append(live, pr)
		}
	}

	if len(live) == 0 {
		// everyone is cooling down, retry whoever recovers soonest instead of leaking
		soonest := p.proxies[0]
		for _, pr := range p.proxies[1:] {
			if pr.disabledUntil.Before(soonest.disabledUntil) {
				soonest = pr
			}
		}
		soonest.lastUsed = now
		return soonest
	}

	var chosen *Proxy
	switch p.strategy {
	case "random":
		chosen = live[rand.Intn(len(live))]
	case "best":
		chosen = live[0]
		for _, pr := range live[1:] {
			if p.rate(pr) > p.rate(chosen) {
				chosen = pr
			}
		}
	default:
		p.index = (p.index + 1) % len(live)
		chosen = live[p.index]
	}

	chosen.lastUsed = now
	return chosen
}

func (p *Pool) Report(pr *Proxy, ok bool, latency time.Duration) {
	if pr == nil {
		return
	}
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if latency > 0 {
		pr.latencyTotal += latency
		pr.latencyCount++
	}

	pr.window = append(pr.window, ok)
	if len(pr.window) > p.windowSize {
		pr.window = pr.window[len(pr.window)-p.windowSize:]
	}

	if ok {
		pr.ok++
		pr.consecutiveFails = 0
	} else {
		pr.fail++
		pr.consecutiveFails++
		if pr.consecutiveFails >= p.maxFails {
			pr.disabledUntil = time.Now().Add(p.cooldown)
		}
	}
}

func (p *Pool) Snapshot() []ProxyStat {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	now := time.Now()
	out := make([]ProxyStat, 0, len(p.proxies))
	for _, pr := range p.proxies {
		stat := ProxyStat{
			Host:     pr.url.Host,
			Live:     pr.disabledUntil.Before(now),
			OK:       pr.ok,
			Fail:     pr.fail,
			Rate:     p.rate(pr),
			LastUsed: pr.lastUsed,
		}
		if pr.latencyCount > 0 {
			stat.AvgLatency = pr.latencyTotal / time.Duration(pr.latencyCount)
		}
		out = append(out, stat)
	}
	return out
}

func (p *Pool) Size() int {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return len(p.proxies)
}

// rate is successrate over a sliding window, fresh proxy with no history is seen as good so it gets tried
func (p *Pool) rate(pr *Proxy) float64 {
	if len(pr.window) == 0 {
		return 1
	}
	ok := 0
	for _, b := range pr.window {
		if b {
			ok++
		}
	}
	return float64(ok) / float64(len(pr.window))
}

func gatherProxies(config Config) ([]string, error) {
	out := append([]string{}, config.Proxies...)
	if config.ProxyDir == "" {
		return out, nil
	}

	entries, err := os.ReadDir(config.ProxyDir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, fmt.Errorf("failed to read proxy dir: %v", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(config.ProxyDir, e.Name())
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".txt":
			lines, err := readProxyTxt(path)
			if err != nil {
				return nil, err
			}
			out = append(out, lines...)
		case ".json":
			lines, err := readProxyJSON(path)
			if err != nil {
				return nil, err
			}
			out = append(out, lines...)
		}
	}
	return out, nil
}

func readProxyTxt(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %v", path, err)
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, nil
}

func readProxyJSON(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %v", path, err)
	}
	var out []string
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %v", path, err)
	}
	return out, nil
}
