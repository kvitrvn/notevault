package vault

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveNoteConflictWhenRevisionMismatch(t *testing.T) {
	svc, _ := setupVault(t)
	first, err := svc.CreateNote("", "Hello", "")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	opened, err := svc.OpenNoteFile(first.RelativePath)
	if err != nil {
		t.Fatalf("OpenNoteFile: %v", err)
	}
	if opened.Revision == "" {
		t.Fatal("révision vide, attendu un sha256")
	}

	// Modifier le fichier en bypassant le service.
	path := filepath.Join(svc.Root(), filepath.FromSlash(first.RelativePath))
	if err := os.WriteFile(path, []byte("# Hello\n\nmodifié hors app"), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	first.Content = "# Hello\n\nlocal edit"
	if _, err := svc.SaveNote(first, opened.Revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("attendu ErrConflict, obtenu %v", err)
	}

	// Force : passe outre.
	if _, err := svc.SaveNoteForce(first); err != nil {
		t.Fatalf("SaveNoteForce: %v", err)
	}
	if _, err := svc.OpenNote(first.RelativePath); err != nil {
		t.Fatalf("OpenNote: %v", err)
	}
}

func TestSaveNoteNoConflictWhenRevisionMatches(t *testing.T) {
	svc, _ := setupVault(t)
	first, err := svc.CreateNote("", "Hello", "")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	opened, err := svc.OpenNoteFile(first.RelativePath)
	if err != nil {
		t.Fatalf("OpenNoteFile: %v", err)
	}
	first.Content = "# Hello\n\nup to date"
	if _, err := svc.SaveNote(first, opened.Revision); err != nil {
		t.Fatalf("SaveNote: %v", err)
	}
}

func TestSaveNoteEmptyRevisionAllowsWrite(t *testing.T) {
	svc, _ := setupVault(t)
	first, err := svc.CreateNote("", "Hello", "")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	first.Content = "# Hello\n\nno expectation"
	if _, err := svc.SaveNote(first, ""); err != nil {
		t.Fatalf("SaveNote(empty): %v", err)
	}
}

func TestOpenNoteFileRevisionStable(t *testing.T) {
	svc, _ := setupVault(t)
	first, err := svc.CreateNote("", "Stable", "")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	r1, err := svc.OpenNoteFile(first.RelativePath)
	if err != nil {
		t.Fatalf("OpenNoteFile 1: %v", err)
	}
	// Sans modification, deux lectures doivent donner la même révision.
	r2, err := svc.OpenNoteFile(first.RelativePath)
	if err != nil {
		t.Fatalf("OpenNoteFile 2: %v", err)
	}
	if r1.Revision != r2.Revision {
		t.Fatalf("révision instable: %s vs %s", r1.Revision, r2.Revision)
	}
	if err := svc.IndexNow(context.Background(), nil); err != nil {
		t.Fatalf("IndexNow: %v", err)
	}
}

func TestRescanVault(t *testing.T) {
	svc, dir := setupVault(t)
	capture := &captureHandler{}
	svc.OnChange(capture.Callback)

	external := filepath.Join(dir, "notes", "inbox", "fresh.md")
	if err := os.MkdirAll(filepath.Dir(external), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(external, []byte("# Fresh\n\nbody"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := svc.RescanVault(); err != nil {
		t.Fatalf("RescanVault: %v", err)
	}

	if !capture.HasFullRescan() {
		t.Fatalf("RescanVault doit publier FullRescan=true")
	}
	if _, err := svc.index.Get("notes/inbox/fresh.md"); err != nil {
		t.Fatalf("index.Get fresh.md: %v", err)
	}
}
