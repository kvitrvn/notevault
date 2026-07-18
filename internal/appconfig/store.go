// Package appconfig persists application-wide state outside any vault.
package appconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	CurrentVersion      = 2
	MaxRecent           = 8
	maxModelRunes       = 120
	defaultChatProvider = "ollama"
)

var chatProviders = [...]string{"ollama", "openai", "mistral", "openrouter"}

type RecentVault struct {
	Path         string    `json:"path"`
	LastOpenedAt time.Time `json:"lastOpenedAt"`
}

type Config struct {
	Version             int               `json:"version"`
	ActiveVault         string            `json:"activeVault,omitempty"`
	RecentVaults        []RecentVault     `json:"recentVaults,omitempty"`
	OnboardingDismissed bool              `json:"onboardingDismissed"`
	ChatProvider        string            `json:"chatProvider"`
	ChatModels          map[string]string `json:"chatModels"`
}

func Default() Config {
	models := make(map[string]string, len(chatProviders))
	for _, provider := range chatProviders {
		models[provider] = ""
	}
	return Config{
		Version:      CurrentVersion,
		RecentVaults: []RecentVault{},
		ChatProvider: defaultChatProvider,
		ChatModels:   models,
	}
}

type Store struct {
	path string
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("déterminer le dossier de configuration : %w", err)
	}
	return filepath.Join(dir, "NoteVault", "app.json"), nil
}

func NewStore(path string) *Store { return &Store{path: path} }

func (s *Store) Path() string { return s.path }

func (s *Store) Load() (Config, bool, error) {
	cfg := Default()
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, false, nil
	}
	if err != nil {
		return cfg, false, fmt.Errorf("lire la configuration globale : %w", err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, true, fmt.Errorf("décoder la configuration globale : %w", err)
	}
	loadedVersion := cfg.Version
	cfg.Normalize()
	if loadedVersion != CurrentVersion {
		if err := s.Save(cfg); err != nil {
			return cfg, true, fmt.Errorf("migrer la configuration globale : %w", err)
		}
	}
	return cfg, true, nil
}

func (s *Store) Save(cfg Config) error {
	cfg.Normalize()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("préparer la configuration globale : %w", err)
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encoder la configuration globale : %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), "app.json.tmp-*")
	if err != nil {
		return fmt.Errorf("créer la configuration temporaire : %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("protéger la configuration temporaire : %w", err)
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("écrire la configuration temporaire : %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("synchroniser la configuration temporaire : %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("fermer la configuration temporaire : %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("remplacer la configuration globale : %w", err)
	}
	return nil
}

func (c *Config) Normalize() {
	c.Version = CurrentVersion
	if !knownChatProvider(c.ChatProvider) {
		c.ChatProvider = defaultChatProvider
	}
	models := make(map[string]string, len(chatProviders))
	for _, provider := range chatProviders {
		model := strings.TrimSpace(c.ChatModels[provider])
		if utf8.RuneCountInString(model) > maxModelRunes {
			model = ""
		}
		models[provider] = model
	}
	c.ChatModels = models
	seen := make(map[string]int, len(c.RecentVaults))
	recent := make([]RecentVault, 0, len(c.RecentVaults))
	for _, item := range c.RecentVaults {
		if item.Path == "" {
			continue
		}
		path := filepath.Clean(item.Path)
		if index, ok := seen[path]; ok {
			if item.LastOpenedAt.After(recent[index].LastOpenedAt) {
				item.Path = path
				recent[index] = item
			}
			continue
		}
		seen[path] = len(recent)
		item.Path = path
		recent = append(recent, item)
	}
	sort.SliceStable(recent, func(i, j int) bool {
		return recent[i].LastOpenedAt.After(recent[j].LastOpenedAt)
	})
	if len(recent) > MaxRecent {
		recent = recent[:MaxRecent]
	}
	c.RecentVaults = recent
}

func knownChatProvider(provider string) bool {
	for _, known := range chatProviders {
		if provider == known {
			return true
		}
	}
	return false
}

func (c *Config) RecordOpen(path string, openedAt time.Time) {
	path = filepath.Clean(path)
	c.ActiveVault = path
	next := []RecentVault{{Path: path, LastOpenedAt: openedAt.UTC()}}
	for _, item := range c.RecentVaults {
		if filepath.Clean(item.Path) != path {
			next = append(next, item)
		}
	}
	c.RecentVaults = next
	c.Normalize()
}

func (c *Config) Forget(path string) {
	path = filepath.Clean(path)
	next := make([]RecentVault, 0, len(c.RecentVaults))
	for _, item := range c.RecentVaults {
		if filepath.Clean(item.Path) != path {
			next = append(next, item)
		}
	}
	c.RecentVaults = next
}
