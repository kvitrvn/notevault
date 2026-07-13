package appconfig

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRoundTripAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "NoteVault", "app.json")
	store := NewStore(path)
	cfg := Default()
	cfg.RecordOpen("/vault/a", time.Unix(10, 0))
	cfg.OnboardingDismissed = true
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %o, want 600", info.Mode().Perm())
	}
	got, exists, err := store.Load()
	if err != nil || !exists {
		t.Fatalf("Load() = %+v, %v, %v", got, exists, err)
	}
	if got.ActiveVault != "/vault/a" || !got.OnboardingDismissed {
		t.Fatalf("configuration perdue: %+v", got)
	}
}

func TestConfigRecentDeduplicatedSortedLimitedAndForgotten(t *testing.T) {
	cfg := Default()
	for i := 0; i < 10; i++ {
		cfg.RecordOpen(filepath.Join("/vault", string(rune('a'+i))), time.Unix(int64(i), 0))
	}
	cfg.RecordOpen("/vault/e", time.Unix(20, 0))
	if len(cfg.RecentVaults) != MaxRecent {
		t.Fatalf("len = %d, want %d", len(cfg.RecentVaults), MaxRecent)
	}
	if cfg.RecentVaults[0].Path != "/vault/e" {
		t.Fatalf("premier = %+v", cfg.RecentVaults[0])
	}
	cfg.Forget("/vault/e")
	if cfg.ActiveVault != "/vault/e" {
		t.Fatalf("le coffre actif ne doit pas être fermé: %q", cfg.ActiveVault)
	}
	for _, item := range cfg.RecentVaults {
		if item.Path == "/vault/e" {
			t.Fatal("entrée oubliée encore présente")
		}
	}
}

func TestConfigNormalizeKeepsNewestDuplicate(t *testing.T) {
	cfg := Config{RecentVaults: []RecentVault{
		{Path: "/vault/a", LastOpenedAt: time.Unix(1, 0)},
		{Path: "/vault/a", LastOpenedAt: time.Unix(3, 0)},
		{Path: "/vault/b", LastOpenedAt: time.Unix(2, 0)},
	}}
	cfg.Normalize()
	if len(cfg.RecentVaults) != 2 || cfg.RecentVaults[0].Path != "/vault/a" || cfg.RecentVaults[0].LastOpenedAt.Unix() != 3 {
		t.Fatalf("normalisation = %+v", cfg.RecentVaults)
	}
}
