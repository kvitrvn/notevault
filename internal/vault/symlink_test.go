package vault

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Un coffre synchronisé (Dropbox, Syncthing) ou une archive extraite peut
// déposer un lien symbolique dans notes/. Le contenu visé se trouve hors du
// coffre : la lecture doit être refusée, pas suivie.
func TestOpenNoteRefusesSymlinkEscapingVault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("les liens symboliques demandent des privilèges sous Windows")
	}
	root := t.TempDir()
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer svc.Close()

	secret := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(secret, []byte("# Hors du coffre\n\ncontenu privé"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	link := filepath.Join(root, "notes", "piege.md")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlink indisponible : %v", err)
	}

	note, err := svc.OpenNote("notes/piege.md")
	if err == nil {
		t.Fatalf("OpenNote a suivi le lien symbolique et renvoyé %q", note.Content)
	}
	if strings.Contains(err.Error(), "contenu privé") {
		t.Fatalf("l'erreur divulgue le contenu visé : %v", err)
	}
}

// Le même refus doit valoir pour un lien vers un fichier interne au coffre :
// os.Root ne suit aucun lien, ce qui garde la règle simple et vérifiable.
func TestReadVaultFileRefusesSymlinkWithinVault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("les liens symboliques demandent des privilèges sous Windows")
	}
	root := t.TempDir()
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer svc.Close()

	target := filepath.Join(root, "notes", "reelle.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(target, []byte("# Réelle"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(root, "notes", "alias.md")); err != nil {
		t.Skipf("symlink indisponible : %v", err)
	}

	if _, err := svc.readVaultFile("notes/alias.md"); err == nil {
		t.Fatal("readVaultFile a suivi un lien symbolique interne")
	}
	if _, err := svc.readVaultFile("notes/reelle.md"); err != nil {
		t.Fatalf("readVaultFile sur un fichier ordinaire : %v", err)
	}
}
