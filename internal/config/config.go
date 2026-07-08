// Package config centralise la configuration persistée du coffre.
// Les valeurs sont stockées dans <root>/.notevault/config.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	fileName = "config.json"
	dirName  = ".notevault"
)

type Config struct {
	VaultPath          string `json:"vaultPath"`
	Theme              string `json:"theme"`
	AutoSaveDebounceMs int    `json:"autoSaveDebounceMs"`
	TrashRetentionDays int    `json:"trashRetentionDays"`
	HistoryPerNote     int    `json:"historyPerNote"`
	AutoDailyNote      bool   `json:"autoDailyNote"`
}

func Default() Config {
	return Config{
		VaultPath:          "",
		Theme:              "dark",
		AutoSaveDebounceMs: 1500,
		TrashRetentionDays: 30,
		HistoryPerNote:     30,
		AutoDailyNote:      false,
	}
}

func (c *Config) Normalize() {
	if c.Theme != "light" && c.Theme != "dark" {
		c.Theme = "dark"
	}
	if c.AutoSaveDebounceMs <= 0 {
		c.AutoSaveDebounceMs = 1500
	}
	if c.TrashRetentionDays <= 0 {
		c.TrashRetentionDays = 30
	}
	if c.HistoryPerNote <= 0 {
		c.HistoryPerNote = 30
	}
}

// Store lit et écrit la configuration depuis le coffre.
// Une seule instance par coffre ; les méthodes sont sûres pour un usage
// mono-utilisateur (le frontend Wails est mono-threadé pour les appels).
type Store struct {
	path string
}

func NewStore(vaultRoot string) *Store {
	return &Store{path: filepath.Join(vaultRoot, dirName, fileName)}
}

func (s *Store) Load() (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg.VaultPath = filepath.Dir(filepath.Dir(s.path))
			return cfg, nil
		}
		return cfg, fmt.Errorf("lire la configuration : %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("decoder la configuration : %w", err)
	}
	cfg.Normalize()
	return cfg, nil
}

func (s *Store) Save(cfg Config) error {
	cfg.Normalize()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("préparer le dossier de config : %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encoder la configuration : %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("écrire la configuration (tmp) : %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("renommer la configuration : %w", err)
	}
	return nil
}

func (s *Store) Path() string {
	return s.path
}
