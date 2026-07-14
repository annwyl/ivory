package stores

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/annwyl/ivory"
)

func newStore(t *testing.T, name, path string) ivory.Store {
	t.Helper()
	config, _ := json.Marshal(path)
	store, err := ivory.GetRegisteredStores()[name](config)
	if err != nil {
		t.Fatalf("failed to create %s store: %v", name, err)
	}
	return store
}

func TestJSONLStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	store := newStore(t, "jsonl", path)

	if err := store.Save("1", map[string]any{"title": "hello", "id": 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"title":"hello"`) {
		t.Fatalf("record not written correctly: %s", data)
	}
}

func TestJSONLStoreDedups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	store := newStore(t, "jsonl", path)

	store.Save("dup", map[string]any{"n": 1})
	store.Save("dup", map[string]any{"n": 2})
	store.Close()

	data, _ := os.ReadFile(path)
	if lines := strings.Count(strings.TrimSpace(string(data)), "\n"); lines != 0 {
		t.Fatalf("expected a single line for one key, got %d extra", lines)
	}
}

func TestJSONLStoreQuery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	store := newStore(t, "jsonl", path)
	defer store.Close()

	store.Save("1", map[string]any{"title": "go rocks"})
	store.Save("2", map[string]any{"title": "rust also good"})

	got, err := store.(ivory.Queryable).Query("rust", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0]["title"] != "rust also good" {
		t.Fatalf("unexpected query result: %v", got)
	}
}

func TestSQLiteStoreQuery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.db")
	store := newStore(t, "sqlite", path)
	defer store.Close()

	store.Save("1", map[string]any{"score": 10})
	store.Save("2", map[string]any{"score": 20})

	got, err := store.(ivory.Queryable).Query("20", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one match, got %d", len(got))
	}
}

func TestSQLiteStoreUpserts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.db")
	store := newStore(t, "sqlite", path)

	store.Save("42", map[string]any{"score": 10})
	store.Save("42", map[string]any{"score": 20})
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var count int
	if err := db.QueryRow("select count(*) from records").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one row after upsert, got %d", count)
	}
}
