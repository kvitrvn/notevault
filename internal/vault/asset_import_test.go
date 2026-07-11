package vault

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestImportAssetFromFilePathOK(t *testing.T) {
	root := t.TempDir()
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer svc.Close()

	src := filepath.Join(t.TempDir(), "source.png")
	payload := []byte{0x89, 0x50, 0x4E, 0x47}
	if err := os.WriteFile(src, payload, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rel, err := svc.ImportAssetFromFilePath(src)
	if err != nil {
		t.Fatalf("ImportAssetFromFilePath: %v", err)
	}
	if !strings.HasPrefix(rel, "assets/") || !strings.HasSuffix(rel, ".png") {
		t.Fatalf("rel inattendu: %q", rel)
	}
	dest := filepath.Join(root, rel)
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile dest: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload mismatch")
	}
}

func TestImportAssetFromFilePathBadExt(t *testing.T) {
	root := t.TempDir()
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer svc.Close()

	src := filepath.Join(t.TempDir(), "evil.exe")
	if err := os.WriteFile(src, []byte("MZ"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := svc.ImportAssetFromFilePath(src); err == nil {
		t.Fatalf("attendu erreur pour .exe")
	}
}

func TestImportAssetFromFilePathMissing(t *testing.T) {
	root := t.TempDir()
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer svc.Close()
	if _, err := svc.ImportAssetFromFilePath("/nonexistent/path.png"); err == nil {
		t.Fatalf("attendu erreur fichier introuvable")
	}
}

func TestImportAssetFromFilePathDir(t *testing.T) {
	root := t.TempDir()
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer svc.Close()
	dir := t.TempDir()
	if _, err := svc.ImportAssetFromFilePath(dir); err == nil {
		t.Fatalf("attendu erreur pour un dossier")
	}
}

func TestServiceResolveAsset(t *testing.T) {
	service, _ := setupVault(t)
	rel, err := service.SaveAsset([]byte("image"), "photo.png")
	if err != nil {
		t.Fatal(err)
	}

	abs, err := service.ResolveAsset(rel)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(abs) {
		t.Fatalf("ResolveAsset() = %q, want absolute path", abs)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("resolved asset: %v", err)
	}
}

func TestServiceResolveAssetRejectsInvalidPaths(t *testing.T) {
	service, _ := setupVault(t)
	tests := []struct {
		name string
		path string
	}{
		{name: "traversal", path: "assets/../../outside.png"},
		{name: "note", path: "notes/private.png"},
		{name: "absolute", path: filepath.Join(string(filepath.Separator), "tmp", "outside.png")},
		{name: "unsupported extension", path: "assets/file.exe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := service.ResolveAsset(tt.path); err == nil {
				t.Fatalf("ResolveAsset(%q) succeeded, want error", tt.path)
			}
		})
	}
}

func TestServiceResolveAssetRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("la création de symlinks nécessite souvent des privilèges sous Windows")
	}
	service, _ := setupVault(t)
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(service.Root(), "assets", "escape.png")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	if _, err := service.ResolveAsset("assets/escape.png"); err == nil {
		t.Fatal("ResolveAsset() followed a symlink outside assets")
	}
}
