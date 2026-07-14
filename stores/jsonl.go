package stores

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/annwyl/ivory"
)

func ensureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}

type JSONLStore struct {
	file  *os.File
	seen  map[string]bool
	mutex sync.Mutex
}

func init() {
	err := ivory.RegisterStore("jsonl", func(config json.RawMessage) (ivory.Store, error) {
		var filename string
		if err := json.Unmarshal(config, &filename); err != nil {
			return nil, err
		}

		if err := ensureDir(filename); err != nil {
			return nil, err
		}

		file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		return &JSONLStore{file: file, seen: make(map[string]bool)}, nil
	})
	if err != nil {
		panic(err)
	}
}

func (s *JSONLStore) Save(key string, record map[string]any) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %v", err)
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	if key != "" {
		if s.seen[key] {
			return nil
		}
		s.seen[key] = true
	}
	_, err = fmt.Fprintf(s.file, "%s\n", data)
	return err
}

func (s *JSONLStore) Query(term string, limit int) ([]map[string]any, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	name := s.file.Name()
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %v", name, err)
	}

	term = strings.ToLower(term)
	var out []map[string]any
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		if term != "" && !strings.Contains(strings.ToLower(line), term) {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		out = append(out, record)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *JSONLStore) Close() error {
	return s.file.Close()
}
