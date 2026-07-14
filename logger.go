package ivory

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const maxRecent = 200

type logEntry struct {
	at      time.Time
	level   slog.Level
	crawler string
	message string
}

type Logger struct {
	level   *slog.LevelVar
	slog    *slog.Logger
	console bool
	recent  []logEntry
	file    *os.File
	mutex   sync.Mutex
}

func NewLogger(filename, level string) (*Logger, error) {
	lv := &slog.LevelVar{}
	lv.Set(parseLevel(level))
	l := &Logger{console: true, level: lv}
	if filename == "" {
		return l, nil
	}
	if dir := filepath.Dir(filename); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %v", err)
		}
	}
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %v", err)
	}
	l.file = file
	l.slog = slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{Level: lv}))
	return l, nil
}

func (l *Logger) Info(crawler, message string)  { l.log(slog.LevelInfo, crawler, message) }
func (l *Logger) Warn(crawler, message string)  { l.log(slog.LevelWarn, crawler, message) }
func (l *Logger) Error(crawler, message string) { l.log(slog.LevelError, crawler, message) }

func (l *Logger) SetLevel(level string) {
	l.level.Set(parseLevel(level))
}

func (l *Logger) log(level slog.Level, crawler, message string) {
	if level < l.level.Level() {
		return
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	entry := logEntry{at: time.Now(), level: level, crawler: crawler, message: message}
	if l.console {
		fmt.Println(format(entry))
	}
	if l.slog != nil {
		l.slog.LogAttrs(context.Background(), level, message, slog.String("crawler", crawler))
	}

	l.recent = append(l.recent, entry)
	if len(l.recent) > maxRecent {
		l.recent = l.recent[len(l.recent)-maxRecent:]
	}
}

func (l *Logger) SetConsole(on bool) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.console = on
}

func (l *Logger) Recent(n int) []string {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return tail(l.recent, n, "")
}

func (l *Logger) RecentFor(crawler string, n int) []string {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return tail(l.recent, n, crawler)
}

func (l *Logger) Close() error {
	if l.file == nil {
		return nil
	}
	return l.file.Close()
}

func tail(entries []logEntry, n int, crawler string) []string {
	out := make([]string, 0, n)
	for i := len(entries) - 1; i >= 0 && len(out) < n; i-- {
		if crawler != "" && entries[i].crawler != crawler {
			continue
		}
		out = append(out, format(entries[i]))
	}
	// michael jackson, flip it so oldest is first
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func format(e logEntry) string {
	return fmt.Sprintf("[%s] %-5s %s >> %s", e.at.Format("15:04:05"), e.level, e.crawler, e.message)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
