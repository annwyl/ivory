package tui

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/annwyl/ivory"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	viewCrawlers = iota
	viewProxies
)

const logoArt = `
 ___ __     __  ___   ____   __   __
|_ _|\ \   / / / _ \ |  _ \  \ \ / /
 | |  \ \ / / | | | || |_) |  \ V /
 | |   \ V /  | |_| ||  _ <    | |
|___|   \_/    \___/ |_| \_\   |_|  `

var (
	violet = lipgloss.Color("99")

	tableHeader  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")).Background(lipgloss.Color("237"))
	runningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	stoppedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selectedRow  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("60"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	boxStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(violet)
	logoStyle    = lipgloss.NewStyle().Bold(true).Foreground(violet)
	infoLabel    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	infoValue    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	tabOn        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(violet).Padding(0, 1)
	tabOff       = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 1)
	footerBar    = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Background(lipgloss.Color("236"))
	filterStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("60")).Padding(0, 1)
	helpBox      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(violet).Padding(1, 3)
)

type model struct {
	engine     *ivory.Engine
	names      []string
	stats      map[string]ivory.CrawlerStats
	proxies    []ivory.ProxyStat
	view       int
	cursor     int
	width      int
	height     int
	refresh    time.Duration
	strategy   string
	showHelp   bool
	detail     bool
	filtering  bool
	filter     string
	configPath string
}

type tickMsg time.Time

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func New(e *ivory.Engine, configPath string) model {
	return model{
		engine:     e,
		names:      e.Loaded(),
		stats:      e.Stats(),
		proxies:    e.ProxyStats(),
		width:      100,
		height:     30,
		refresh:    e.RefreshInterval(),
		strategy:   e.Strategy(),
		configPath: configPath,
	}
}

type editDoneMsg struct{ err error }

// editConfig suspends the tui, opens $EDITOR on the config file, and comes back
func editConfig(path string) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	return tea.ExecProcess(exec.Command(editor, path), func(err error) tea.Msg {
		return editDoneMsg{err}
	})
}

func (m model) Init() tea.Cmd {
	return tick(m.refresh)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tickMsg:
		m.reload()
		return m, tick(m.refresh)

	case editDoneMsg:
		if msg.err == nil {
			m.engine.Reconfigure(m.configPath)
			m.names = m.engine.Loaded()
			m.refresh = m.engine.RefreshInterval()
			m.strategy = m.engine.Strategy()
			m.reload()
		}
		return m, nil

	case tea.KeyMsg:
		if m.filtering {
			return m.filterKey(msg), nil
		}
		return m.key(msg)
	}
	return m, nil
}

func (m model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	case "esc":
		switch {
		case m.showHelp:
			m.showHelp = false
		case m.detail:
			m.detail = false
		case m.filter != "":
			m.filter = ""
		}
		m.reload()
		return m, nil
	}

	if m.showHelp {
		return m, nil
	}

	switch msg.String() {
	case "enter":
		if !m.detail && m.rows() > 0 {
			m.detail = true
		}
	case "/":
		if !m.detail {
			m.filtering = true
		}
	case "e":
		return m, editConfig(m.configPath)
	case "tab":
		m.view = (m.view + 1) % 2
		m.cursor, m.detail = 0, false
	case "1":
		m.view, m.cursor, m.detail = viewCrawlers, 0, false
	case "2":
		m.view, m.cursor, m.detail = viewProxies, 0, false
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < m.rows()-1 {
			m.cursor++
		}
	case "s":
		if name := m.selected(); name != "" {
			m.engine.Start(name)
		}
	case "x":
		if name := m.selected(); name != "" {
			m.engine.Stop(name)
		}
	case "r":
		if name := m.selected(); name != "" {
			m.engine.Reload(name)
		}
	case "a":
		if m.view == viewCrawlers {
			m.engine.StartAll()
		}
	case "X":
		if m.view == viewCrawlers {
			m.engine.StopAll()
		}
	}
	m.reload()
	return m, nil
}

func (m model) filterKey(msg tea.KeyMsg) model {
	switch msg.String() {
	case "esc":
		m.filtering, m.filter = false, ""
	case "enter":
		m.filtering = false
	case "backspace":
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
		}
	default:
		if len(msg.Runes) == 1 {
			m.filter += string(msg.Runes)
		}
	}
	m.cursor = 0
	m.reload()
	return m
}

func (m *model) reload() {
	m.stats = m.engine.Stats()
	m.proxies = m.engine.ProxyStats()
	if n := m.rows(); m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m model) filteredNames() []string {
	if m.filter == "" {
		return m.names
	}
	f := strings.ToLower(m.filter)
	out := make([]string, 0, len(m.names))
	for _, n := range m.names {
		if strings.Contains(strings.ToLower(n), f) {
			out = append(out, n)
		}
	}
	return out
}

func (m model) filteredProxies() []ivory.ProxyStat {
	if m.filter == "" {
		return m.proxies
	}
	f := strings.ToLower(m.filter)
	out := make([]ivory.ProxyStat, 0, len(m.proxies))
	for _, p := range m.proxies {
		if strings.Contains(strings.ToLower(p.Host), f) {
			out = append(out, p)
		}
	}
	return out
}

func (m model) rows() int {
	if m.view == viewProxies {
		return len(m.filteredProxies())
	}
	return len(m.filteredNames())
}

func (m model) selected() string {
	if m.view != viewCrawlers {
		return ""
	}
	names := m.filteredNames()
	if m.cursor < 0 || m.cursor >= len(names) {
		return ""
	}
	return names[m.cursor]
}

func (m model) selectedProxy() (ivory.ProxyStat, bool) {
	ps := m.filteredProxies()
	if m.cursor < 0 || m.cursor >= len(ps) {
		return ivory.ProxyStat{}, false
	}
	return ps[m.cursor], true
}

func (m model) View() string {
	switch {
	case m.showHelp:
		return m.helpView()
	case m.detail:
		return m.detailView()
	}

	contentW := m.width - 2
	header := m.header()
	tabs := m.tabs()
	footer := footerBar.Width(m.width).Render(m.footer())

	top := []string{header, tabs}
	chrome := lipgloss.Height(header) + 3 // tabs, logs label, footer
	if m.filtering || m.filter != "" {
		bar := "/" + m.filter
		if m.filtering {
			bar += "█"
		}
		top = append(top, filterStyle.Render(bar))
		chrome++
	}

	headerLine, rows := m.tableRows(contentW)
	avail := m.height - chrome - 4
	if avail < 6 {
		avail = 6
	}

	body := buildTable(headerLine, rows, m.cursor, avail-4)
	table := boxStyle.Width(contentW).Render(body)

	logHeight := avail - lipgloss.Height(body)
	if logHeight < 1 {
		logHeight = 1
	}
	logs := boxStyle.Width(contentW).Height(logHeight).Render(dimStyle.Render(strings.Join(logsOr(m.engine.Logs(logHeight)), "\n")))

	sections := append(top, table, dimStyle.Render(" logs"), logs, footer)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m model) detailView() string {
	contentW := m.width - 2
	footer := footerBar.Width(m.width).Render("  esc back · s start · x stop · r reload · q quit")

	var title, info, name string
	if m.view == viewProxies {
		p, ok := m.selectedProxy()
		if !ok {
			return "no proxy selected"
		}
		title = lipgloss.JoinHorizontal(lipgloss.Left, tabOn.Render(p.Host), dimStyle.Render(" proxy"))
		info = boxStyle.Width(contentW).Render(strings.Join([]string{
			kv("state", proxyState(p.Live)),
			kv("requests ok", fmt.Sprintf("%d", p.OK)),
			kv("requests failed", fmt.Sprintf("%d", p.Fail)),
			kv("success rate", fmt.Sprintf("%.0f%%", p.Rate*100)),
			kv("avg latency", latency(p.AvgLatency)),
			kv("last used", ago(p.LastUsed)),
		}, "\n"))
	} else {
		name = m.selected()
		ci, ok := m.engine.CrawlerInfo(name)
		if !ok {
			return "no crawler selected"
		}
		title = lipgloss.JoinHorizontal(lipgloss.Left, tabOn.Render(name), dimStyle.Render(" crawler"))
		lines := []string{
			kv("state", stateText(ci.Stats.Running)),
			kv("workers", fmt.Sprintf("%d", ci.Workers)),
			kv("pages", fmt.Sprintf("%d", ci.Stats.Saved)),
			kv("errors", fmt.Sprintf("%d", ci.Stats.Errors)),
			kv("uptime", uptime(ci.Stats.Started, ci.Stats.Running)),
			kv("last run", ago(ci.Stats.LastRun)),
			"",
			kv("timeout", fmt.Sprintf("%ds", ci.Timeout)),
			kv("retries", fmt.Sprintf("%d", ci.Retries)),
			kv("rate limit", fmt.Sprintf("%dms", ci.RateLimit)),
			kv("max concurrent", fmt.Sprintf("%d", ci.MaxConcurrent)),
		}
		for _, k := range sortedKeys(ci.Params) {
			lines = append(lines, kv(k, ci.Params[k]))
		}
		info = boxStyle.Width(contentW).Render(strings.Join(lines, "\n"))
	}

	logHeight := m.height - lipgloss.Height(title) - lipgloss.Height(info) - 5
	if logHeight < 1 {
		logHeight = 1
	}
	var logLines []string
	if name != "" {
		logLines = m.engine.CrawlerLogs(name, logHeight)
	}
	logs := boxStyle.Width(contentW).Height(logHeight).Render(dimStyle.Render(strings.Join(logsOr(logLines), "\n")))

	return lipgloss.JoinVertical(lipgloss.Left, title, dimStyle.Render(" config"), info, dimStyle.Render(" logs"), logs, footer)
}

func (m model) header() string {
	running := 0
	for _, s := range m.stats {
		if s.Running {
			running++
		}
	}
	live := 0
	for _, p := range m.proxies {
		if p.Live {
			live++
		}
	}

	info := lipgloss.JoinVertical(lipgloss.Left,
		infoLine("Crawlers", fmt.Sprintf("%d", len(m.names))),
		infoLine("Running", fmt.Sprintf("%d", running)),
		infoLine("Proxies", fmt.Sprintf("%d/%d live", live, len(m.proxies))),
		infoLine("Strategy", m.strategy),
		infoLine("Refresh", m.refresh.String()),
	)

	logo := logoStyle.Render(strings.Trim(logoArt, "\n"))
	gap := m.width - lipgloss.Width(info) - lipgloss.Width(logo)
	if gap < 3 {
		return info
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, info, strings.Repeat(" ", gap), logo)
}

func (m model) tabs() string {
	live := 0
	for _, p := range m.proxies {
		if p.Live {
			live++
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Left,
		tabStyle(m.view == viewCrawlers).Render(fmt.Sprintf("crawlers %d", len(m.names))),
		tabStyle(m.view == viewProxies).Render(fmt.Sprintf("proxies %d/%d", live, len(m.proxies))),
	)
}

func (m model) tableRows(contentW int) (string, []string) {
	if m.view == viewProxies {
		return m.proxyRows(contentW)
	}
	return m.crawlerRows(contentW)
}

func (m model) crawlerRows(contentW int) (string, []string) {
	stateW, pagesW, errW, lastW, upW := 9, 7, 7, 10, 9
	nameW := contentW - stateW - pagesW - errW - lastW - upW - 5
	if nameW < 8 {
		nameW = 8
	}
	widths := []int{nameW, stateW, pagesW, errW, lastW, upW}

	header := tableHeader.Width(contentW).Render(rowLine(widths, []string{"NAME", "STATE", "PAGES", "ERRORS", "LAST", "UPTIME"}))
	names := m.filteredNames()
	rows := make([]string, 0, len(names))
	for i, name := range names {
		s := m.stats[name]
		values := []string{name, stateText(s.Running), fmt.Sprintf("%d", s.Saved), fmt.Sprintf("%d", s.Errors), ago(s.LastRun), uptime(s.Started, s.Running)}
		rows = append(rows, m.styleRow(rowLine(widths, values), i, stateText(s.Running), s.Running))
	}
	return header, rows
}

func (m model) proxyRows(contentW int) (string, []string) {
	proxies := m.filteredProxies()
	if len(proxies) == 0 {
		return dimStyle.Render("no proxies configured. add a proxies/ folder with .txt or .json files."), nil
	}

	stateW, okW, failW, rateW, lastW, latW := 9, 6, 6, 6, 10, 8
	hostW := contentW - stateW - okW - failW - rateW - lastW - latW - 6
	if hostW < 10 {
		hostW = 10
	}
	widths := []int{hostW, stateW, okW, failW, rateW, lastW, latW}

	header := tableHeader.Width(contentW).Render(rowLine(widths, []string{"HOST", "STATE", "OK", "FAIL", "RATE", "LAST", "LATENCY"}))
	rows := make([]string, 0, len(proxies))
	for i, p := range proxies {
		values := []string{p.Host, proxyState(p.Live), fmt.Sprintf("%d", p.OK), fmt.Sprintf("%d", p.Fail), fmt.Sprintf("%.0f%%", p.Rate*100), ago(p.LastUsed), latency(p.AvgLatency)}
		rows = append(rows, m.styleRow(rowLine(widths, values), i, proxyState(p.Live), p.Live))
	}
	return header, rows
}

func (m model) styleRow(line string, i int, word string, ok bool) string {
	if i == m.cursor {
		return selectedRow.Width(lipgloss.Width(line)).Render(line)
	}
	return colorWord(line, word, ok)
}

func (m model) footer() string {
	if m.view == viewProxies {
		return "  tab views · / filter · enter detail · e edit · ? help · q quit"
	}
	return "  tab views · / filter · enter detail · s start · x stop · r reload · e edit · ? help · q quit"
}

func (m model) helpView() string {
	lines := []string{
		infoValue.Render("Ivory — keys"),
		"",
		"  tab / 1 / 2   switch between crawlers and proxies",
		"  up / down     move the cursor",
		"  enter         open the detail view for the selection",
		"  /             filter the list, esc to clear",
		"  e             edit config.json in $EDITOR and reload",
		"  s / x / r     start / stop / reload the selected crawler",
		"  a / X         start all / stop all",
		"  ?             toggle this help",
		"  q             quit",
	}
	box := helpBox.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func buildTable(headerLine string, rows []string, cursor, budget int) string {
	if budget < 2 {
		budget = 2
	}
	lines := []string{headerLine}
	if len(rows) <= budget-1 {
		return strings.Join(append(lines, rows...), "\n")
	}

	visible := budget - 2 // header + scroll hint
	if visible < 1 {
		visible = 1
	}
	start := cursor - visible/2
	if start < 0 {
		start = 0
	}
	if start > len(rows)-visible {
		start = len(rows) - visible
	}

	lines = append(lines, rows[start:start+visible]...)
	lines = append(lines, dimStyle.Render(fmt.Sprintf("  ↑ %d more    ↓ %d more", start, len(rows)-start-visible)))
	return strings.Join(lines, "\n")
}

func logsOr(lines []string) []string {
	if len(lines) == 0 {
		return []string{"no activity yet"}
	}
	return lines
}

func rowLine(widths []int, values []string) string {
	var b strings.Builder
	for i, w := range widths {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%-*s", w, truncate(values[i], w))
	}
	return b.String()
}

func kv(label, value string) string {
	return infoLabel.Render(fmt.Sprintf("%-16s", label)) + " " + infoValue.Render(value)
}

func infoLine(label, value string) string {
	return infoLabel.Render(fmt.Sprintf("%-9s", label)) + " " + infoValue.Render(value)
}

func tabStyle(active bool) lipgloss.Style {
	if active {
		return tabOn
	}
	return tabOff
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func colorWord(line, word string, ok bool) string {
	style := stoppedStyle
	if ok {
		style = runningStyle
	}
	return strings.Replace(line, word, style.Render(word), 1)
}

func stateText(running bool) string {
	if running {
		return "running"
	}
	return "stopped"
}

func proxyState(live bool) string {
	if live {
		return "live"
	}
	return "cooldown"
}

func truncate(s string, w int) string {
	if len(s) <= w {
		return s
	}
	if w <= 1 {
		return s[:w]
	}
	return s[:w-1] + "…"
}

func ago(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return short(time.Since(t)) + " ago"
}

func uptime(started time.Time, running bool) string {
	if !running || started.IsZero() {
		return "-"
	}
	return short(time.Since(started))
}

func latency(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func short(d time.Duration) string {
	d = d.Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

func Run(e *ivory.Engine, configPath string) error {
	program := tea.NewProgram(New(e, configPath), tea.WithAltScreen())
	_, err := program.Run()
	return err
}
