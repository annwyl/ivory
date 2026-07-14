package ivory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		ok   bool
	}{
		{"valid", Config{Store: "jsonl", Timeout: 10, Retries: 2, RateLimit: 100, Crawlers: []string{"x"}}, true},
		{"no store", Config{Timeout: 10, Crawlers: []string{"x"}}, false},
		{"zero timeout", Config{Store: "jsonl", Timeout: 0, Crawlers: []string{"x"}}, false},
		{"negative retries", Config{Store: "jsonl", Timeout: 5, Retries: -1, Crawlers: []string{"x"}}, false},
		{"no crawlers", Config{Store: "jsonl", Timeout: 5}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateConfig(c.cfg)
			if (err == nil) != c.ok {
				t.Fatalf("validateConfig returned %v, expected ok=%v", err, c.ok)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	content := `{"store":"jsonl","store_config":"out.jsonl","timeout":15,"retries":3,"rate_limit":200,"crawlers":["hackernews"]}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	config, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if config.Store != "jsonl" || config.Timeout != 15 || len(config.Crawlers) != 1 {
		t.Fatalf("config not loaded correctly: %+v", config)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil {
		t.Fatal("expected an error for missing file")
	}
}
