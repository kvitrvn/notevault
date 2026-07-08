package vault

import (
	"os"
	"path/filepath"
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
