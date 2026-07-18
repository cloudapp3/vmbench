package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type PersistedConfig struct {
	Theme      string `json:"theme,omitempty"`
	LastMode   string `json:"last_mode,omitempty"`
	LastEngine string `json:"last_engine,omitempty"`
}

func configPath() string {
	if v := os.Getenv("VMBENCH_CONFIG"); v != "" {
		return v
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "vmbench", "config.json")
}

func LoadConfig() PersistedConfig {
	var cfg PersistedConfig
	p := configPath()
	if p == "" {
		return cfg
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func SaveConfig(cfg PersistedConfig) error {
	p := configPath()
	if p == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
