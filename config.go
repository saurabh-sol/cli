package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config is persisted at ~/.gocode/config.json.
type Config struct {
	APIKey      string `json:"api_key"`
	Model       string `json:"model"`
	Effort      string `json:"effort"`
	AutoApprove bool   `json:"auto_approve"`
}

const defaultModel = "claude-opus-5-5"

func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".gocode")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func historyPath() string {
	dir, err := configDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "history")
}

func loadConfig() (*Config, error) {
	cfg := &Config{Model: defaultModel, Effort: "high"}
	p, err := configPath()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return cfg, err
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	if cfg.Effort == "" {
		cfg.Effort = "high"
	}
	return cfg, nil
}

func saveConfig(cfg *Config) error {
	p, err := configPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// ---------------------------------------------------------------------------
// Model catalog
// ---------------------------------------------------------------------------

type modelInfo struct {
	ID        string
	Name      string
	Desc      string
	InPrice   float64 // $ per 1M input tokens
	OutPrice  float64 // $ per 1M output tokens
	CacheRead float64 // $ per 1M cache-read tokens
	Adaptive  bool    // supports thinking {type: adaptive}
	Effort    bool    // supports output_config.effort
}

var knownModels = []modelInfo{
	{"claude-opus-5-5", "Claude Opus 5.5", "Default. Best balance of intelligence, speed and cost.", 4, 20, 0.20, true, true},
	{"claude-fable-5-1", "Claude Fable 5.1", "Most capable model for the hardest reasoning and long agentic work.", 10, 50, 0.25, true, true},
	{"claude-sonnet-5-5", "Claude Sonnet 5.5", "Fast and capable for everyday coding.", 2, 10, 0.20, true, true},
	{"claude-haiku-5-5", "Claude Haiku 5.5", "Fastest and cheapest; great for simple tasks.", 0.10, 0.50, 0.01, true, true},
	{"claude-opus-5", "Claude Opus 5", "Previous Opus generation.", 5, 25, 0.50, true, true},
	{"claude-sonnet-5", "Claude Sonnet 5", "Previous Sonnet generation.", 2, 10, 0.20, true, true},
	{"claude-opus-4-8", "Claude Opus 4.8", "Opus 4.x family.", 5, 25, 0.50, true, true},
	{"claude-opus-4-7", "Claude Opus 4.7", "Opus 4.x family.", 5, 25, 0.50, true, true},
	{"claude-opus-4-6", "Claude Opus 4.6", "Opus 4.x family.", 5, 25, 0.50, true, true},
	{"claude-sonnet-4-6", "Claude Sonnet 4.6", "Sonnet 4.x family.", 3, 15, 0.30, true, true},
	{"claude-haiku-4-5", "Claude Haiku 4.5", "Older Haiku (no adaptive thinking).", 1, 5, 0.10, false, false},
}

func lookupModel(id string) modelInfo {
	for _, m := range knownModels {
		if m.ID == id {
			return m
		}
	}
	// Unknown model: be conservative. Omit thinking/effort so the request is valid
	// on any model; the user can still set /effort which we then send.
	return modelInfo{ID: id, Name: id, Desc: "Custom model id", Adaptive: false, Effort: false}
}

var validEfforts = []string{"low", "medium", "high", "xhigh", "max"}

func isValidEffort(e string) bool {
	for _, v := range validEfforts {
		if v == e {
			return true
		}
	}
	return false
}
