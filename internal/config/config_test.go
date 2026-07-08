package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Theme != "dark" || cfg.AutoSaveDebounceMs != 1500 {
		t.Fatalf("valeurs par défaut incorrectes : %+v", cfg)
	}
	if cfg.VaultPath == "" {
		t.Fatalf("VaultPath par défaut vide")
	}
}

func TestStoreSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	cfg := Default()
	cfg.Theme = "light"
	cfg.AutoDailyNote = true
	cfg.HistoryPerNote = 7
	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	cfg2, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg2.Theme != "light" || !cfg2.AutoDailyNote || cfg2.HistoryPerNote != 7 {
		t.Fatalf("config non restaurée : %+v", cfg2)
	}
}

func TestStoreNormalize(t *testing.T) {
	cfg := Default()
	cfg.Theme = "violet"
	cfg.AutoSaveDebounceMs = -1
	cfg.HistoryPerNote = 0
	cfg.TrashRetentionDays = 0
	cfg.Normalize()
	if cfg.Theme != "dark" {
		t.Fatalf("Theme après normalize : %s", cfg.Theme)
	}
	if cfg.AutoSaveDebounceMs != 1500 || cfg.HistoryPerNote != 30 || cfg.TrashRetentionDays != 30 {
		t.Fatalf("défauts non appliqués : %+v", cfg)
	}
}

func TestStoreAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if err := store.Save(Default()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".notevault", "*.tmp-*"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("tmp résiduel : %v", matches)
	}
	if _, err := os.Stat(store.Path()); err != nil {
		t.Fatalf("config.json absent : %v", err)
	}
}
