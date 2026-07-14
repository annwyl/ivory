package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/annwyl/ivory"
	tea "github.com/charmbracelet/bubbletea"
)

type noopCrawler struct{ name string }

func (c noopCrawler) Name() string { return c.name }

func (c noopCrawler) Run(ctx context.Context, rt *ivory.Runtime) error {
	<-ctx.Done()
	return nil
}

func init() {
	ivory.RegisterCrawler("alpha", func() ivory.Crawler { return noopCrawler{"alpha"} })
	ivory.RegisterCrawler("beta", func() ivory.Crawler { return noopCrawler{"beta"} })
	ivory.RegisterStore("tuimem", func(json.RawMessage) (ivory.Store, error) { return tuimem{}, nil })
}

type tuimem struct{}

func (tuimem) Save(string, map[string]any) error { return nil }
func (tuimem) Close() error                      { return nil }

func newEngine(t *testing.T) *ivory.Engine {
	t.Helper()
	e, err := ivory.NewEngine(ivory.Config{Store: "tuimem", Timeout: 5, Crawlers: []string{"alpha", "beta"}})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func key(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestModelNavigation(t *testing.T) {
	e := newEngine(t)
	defer e.Close()
	m := New(e, "")

	if m.cursor != 0 {
		t.Fatalf("expected cursor at 0, got %d", m.cursor)
	}

	m = update(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Fatalf("expected cursor at 1 after down, got %d", m.cursor)
	}

	m = update(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Fatalf("cursor should not move past the last row, got %d", m.cursor)
	}

	m = update(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Fatalf("expected cursor back at 0, got %d", m.cursor)
	}
}

func TestModelStartStop(t *testing.T) {
	e := newEngine(t)
	defer e.Close()
	m := New(e, "") // cursor on "alpha"

	m = update(m, key("s"))
	if !m.stats["alpha"].Running {
		t.Fatal("alpha should be running after pressing s")
	}

	m = update(m, key("x"))
	deadline := time.Now().Add(2 * time.Second)
	for e.Status()["alpha"] && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if e.Status()["alpha"] {
		t.Fatal("alpha should be stopped after pressing x")
	}
}

func TestModelSwitchView(t *testing.T) {
	e := newEngine(t)
	defer e.Close()
	m := New(e, "")

	if m.view != viewCrawlers {
		t.Fatal("should start on the crawlers view")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.view != viewProxies {
		t.Fatal("tab should switch to the proxies view")
	}
	// actions are a no op on the proxy view
	if m.selected() != "" {
		t.Fatal("selected should be empty on the proxy view")
	}
}

func TestBuildTableScrolls(t *testing.T) {
	rows := make([]string, 20)
	for i := range rows {
		rows[i] = fmt.Sprintf("row-%d", i)
	}

	out := buildTable("HEADER", rows, 10, 6)
	lines := strings.Split(out, "\n")
	if len(lines) != 6 {
		t.Fatalf("expected 6 lines for budget 6, got %d", len(lines))
	}
	if !strings.Contains(out, "more") {
		t.Fatal("expected a scroll hint when rows overflow")
	}

	out = buildTable("HEADER", rows[:3], 0, 10)
	if strings.Contains(out, "more") {
		t.Fatal("did not expect a scroll hint when everything fits")
	}
}

func TestModelFilter(t *testing.T) {
	e := newEngine(t)
	defer e.Close()
	m := New(e, "")

	m = update(m, key("/"))
	if !m.filtering {
		t.Fatal("/ should enter filter mode")
	}
	m = update(m, key("a"))
	m = update(m, key("l"))

	got := m.filteredNames()
	if len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("expected only alpha to match, got %v", got)
	}
	if m.rows() != 1 {
		t.Fatalf("expected 1 visible row, got %d", m.rows())
	}
}

func update(m model, msg tea.Msg) model {
	next, _ := m.Update(msg)
	return next.(model)
}
