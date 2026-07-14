package ivory

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	LogFile         string          `json:"log_file"`
	LogLevel        string          `json:"log_level"`
	Store           string          `json:"store"`
	StoreConfig     json.RawMessage `json:"store_config"`
	Proxies         []string        `json:"proxies"`
	ProxyDir        string          `json:"proxy_dir"`
	ProxyStrategy   string          `json:"proxy_strategy"`
	ProxyMaxFails   int             `json:"proxy_max_fails"`
	ProxyCooldown   int             `json:"proxy_cooldown"`
	ProxyWindow     int             `json:"proxy_window"`
	UserAgents      []string        `json:"user_agents"`
	Timeout         int             `json:"timeout"`
	Retries         int             `json:"retries"`
	RateLimit       int             `json:"rate_limit"`
	MaxConcurrent   int             `json:"max_concurrent"`
	MaxBodyBytes    int             `json:"max_body_bytes"`
	RefreshInterval int             `json:"refresh_interval"`
	Crawlers        []string        `json:"crawlers"`
	Workers         map[string]int  `json:"workers"`
	StartOnLoad     bool            `json:"start_on_load"`
}

func LoadConfig(filename string) (Config, error) {
	var config Config
	file, err := os.Open(filename)
	if err != nil {
		return config, fmt.Errorf("failed to open config file: %v", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		return config, fmt.Errorf("failed to decode config file: %v", err)
	}

	err = validateConfig(config)
	if err != nil {
		return config, fmt.Errorf("invalid config: %v", err)
	}

	return config, nil
}

func validateConfig(config Config) error {
	var errors []string

	if config.Store == "" {
		errors = append(errors, "no store specified")
	}

	if config.Timeout <= 0 {
		errors = append(errors, "timeout must be greater than 0")
	}

	if config.Retries < 0 {
		errors = append(errors, "retries cannot be negative")
	}

	if config.RateLimit < 0 {
		errors = append(errors, "rate limit cannot be negative")
	}

	if len(config.Crawlers) == 0 {
		errors = append(errors, "no crawlers specified")
	}

	if len(errors) > 0 {
		return fmt.Errorf("config validation failed: %s", strings.Join(errors, "; "))
	}

	return nil
}
