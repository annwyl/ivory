package stores

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/annwyl/ivory"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func init() {
	err := ivory.RegisterStore("sqlite", func(config json.RawMessage) (ivory.Store, error) {
		var path string
		if err := json.Unmarshal(config, &path); err != nil {
			return nil, err
		}

		if err := ensureDir(path); err != nil {
			return nil, err
		}

		// WAL and a busy timeout so concurrent workers dont trip over locked db
		db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
		if err != nil {
			return nil, err
		}

		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS records (
			key TEXT PRIMARY KEY,
			data TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`)
		if err != nil {
			return nil, fmt.Errorf("failed to create table: %v", err)
		}

		return &SQLiteStore{db: db}, nil
	})
	if err != nil {
		panic(err)
	}
}

func (s *SQLiteStore) Save(key string, record map[string]any) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %v", err)
	}

	if key == "" {
		key = fmt.Sprintf("%x", sha256.Sum256(data))
	}

	now := time.Now().Format(time.RFC3339)
	// bind as string so the column is TEXT and LIKE queries work (a []byte binds as a BLOB which LIKE won't match)
	_, err = s.db.Exec(`INSERT INTO records (key, data, created_at, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`,
		key, string(data), now, now)
	return err
}

func (s *SQLiteStore) Query(term string, limit int) ([]map[string]any, error) {
	query := "SELECT data FROM records WHERE data LIKE ? ORDER BY updated_at DESC"
	args := []any{"%" + term + "%"}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(data), &record); err != nil {
			continue
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
