package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kvitrvn/notevault/internal/vault"
)

func setupAppForTest(t *testing.T) *App {
	t.Helper()
	service, err := vault.New(vault.Options{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("vault.New: %v", err)
	}
	t.Cleanup(func() { _ = service.Close() })
	return &App{vault: service, assetPort: 43125}
}

func TestAppAssetURL(t *testing.T) {
	app := setupAppForTest(t)
	path := filepath.Join(app.vault.Root(), "assets", "mes photos", "image.png")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := app.AssetURL("assets/mes photos/image.png")
	if err != nil {
		t.Fatal(err)
	}
	want := "http://127.0.0.1:43125/files/assets/mes%20photos/image.png"
	if got != want {
		t.Fatalf("AssetURL() = %q, want %q", got, want)
	}
}

func TestAppAssetMethodsRejectTraversal(t *testing.T) {
	app := setupAppForTest(t)
	outside := filepath.Join(filepath.Dir(app.vault.Root()), "outside.png")
	if err := os.WriteFile(outside, []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}

	for name, call := range map[string]func() error{
		"OpenAsset": func() error {
			_, err := app.OpenAsset("assets/../../outside.png")
			return err
		},
		"AssetURL": func() error {
			_, err := app.AssetURL("assets/../../outside.png")
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil || !strings.Contains(err.Error(), "asset") {
				t.Fatalf("call returned %v, want asset validation error", err)
			}
		})
	}
}

func TestApplicationBackgroundIsOpaque(t *testing.T) {
	app := &App{}
	background := applicationOptions(app).BackgroundColour
	if background == nil {
		t.Fatal("BackgroundColour is nil")
	}
	if background.A != 255 {
		t.Fatalf("background alpha = %d, want 255", background.A)
	}
}
