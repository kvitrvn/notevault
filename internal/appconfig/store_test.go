package appconfig

import (
	"encoding/json"
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
	if got.Version != CurrentVersion || got.ChatProvider != "ollama" || len(got.ChatModels) != 4 {
		t.Fatalf("réglages de chat invalides: %+v", got)
	}
}

func TestStoreMigratesVersionOneChatDefaults(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"onboardingDismissed":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(path)
	cfg, exists, err := store.Load()
	if err != nil || !exists {
		t.Fatalf("Load() = %+v, %v, %v", cfg, exists, err)
	}
	if cfg.Version != 2 || cfg.ChatProvider != "ollama" {
		t.Fatalf("migration = %+v", cfg)
	}
	for _, provider := range chatProviders {
		if cfg.ChatModels[provider] != "" {
			t.Fatalf("modèle %s = %q, want empty", provider, cfg.ChatModels[provider])
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted["version"] != float64(CurrentVersion) {
		t.Fatalf("version persistée = %v", persisted["version"])
	}
	if _, leaked := persisted["apiKey"]; leaked {
		t.Fatal("la configuration contient un champ apiKey")
	}
}

func TestConfigNormalizeRestrictsChatPreferences(t *testing.T) {
	t.Parallel()
	cfg := Config{
		ChatProvider: "custom",
		ChatModels: map[string]string{
			"ollama": "  qwen3:4b  ",
			"openai": string(make([]rune, maxModelRunes+1)),
			"custom": "secret-model",
		},
	}
	cfg.Normalize()
	if cfg.ChatProvider != "ollama" || cfg.ChatModels["ollama"] != "qwen3:4b" || cfg.ChatModels["openai"] != "" {
		t.Fatalf("normalisation chat = %+v", cfg)
	}
	if len(cfg.ChatModels) != len(chatProviders) {
		t.Fatalf("fournisseurs = %+v", cfg.ChatModels)
	}
	if _, exists := cfg.ChatModels["custom"]; exists {
		t.Fatal("fournisseur inconnu conservé")
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
