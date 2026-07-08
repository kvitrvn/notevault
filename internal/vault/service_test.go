package vault

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupVault(t *testing.T) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	svc, err := New(Options{Root: dir, StartWatcher: false})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc, dir
}

func TestServiceCRUD(t *testing.T) {
	svc, _ := setupVault(t)

	created, err := svc.CreateNote("Bonjour", "")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if created.RelativePath == "" {
		t.Fatal("RelativePath vide")
	}

	got, err := svc.OpenNote(created.RelativePath)
	if err != nil {
		t.Fatalf("OpenNote: %v", err)
	}
	if got.Title != "Bonjour" {
		t.Fatalf("Title: %s", got.Title)
	}

	created.Content = "Hello world"
	updated, err := svc.SaveNote(created)
	if err != nil {
		t.Fatalf("SaveNote: %v", err)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) && !updated.UpdatedAt.Equal(created.UpdatedAt) {
		// CreatedAt peut être antérieur d'une nanoseconde, on tolère.
		if updated.UpdatedAt.Before(created.UpdatedAt) {
			t.Fatalf("UpdatedAt doit progresser")
		}
	}

	// Indexation manuelle pour les tests.
	if err := svc.IndexNow(context.Background(), nil); err != nil {
		t.Fatalf("IndexNow: %v", err)
	}
	notes, err := svc.ListNotes()
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("ListNotes: %d entrées", len(notes))
	}

	// Soft delete.
	if err := svc.DeleteNote(created.RelativePath); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	trash, err := svc.ListTrash()
	if err != nil {
		t.Fatalf("ListTrash: %v", err)
	}
	if len(trash) != 1 {
		t.Fatalf("corbeille: %d entrées", len(trash))
	}

	// Restore.
	restored, err := svc.RestoreFromTrash(trash[0].ID)
	if err != nil {
		t.Fatalf("RestoreFromTrash: %v", err)
	}
	if restored.Title != created.Title {
		t.Fatalf("restauré: %s", restored.Title)
	}
}

func TestServiceSearch(t *testing.T) {
	svc, _ := setupVault(t)

	titles := []string{"Vacances été", "TODO listes", "Recette crumble"}
	for _, title := range titles {
		note, err := svc.CreateNote(title, "")
		if err != nil {
			t.Fatalf("CreateNote: %v", err)
		}
		note.Content = "Contenu de " + title
		if _, err := svc.SaveNote(note); err != nil {
			t.Fatalf("SaveNote: %v", err)
		}
	}
	if err := svc.IndexNow(context.Background(), nil); err != nil {
		t.Fatalf("IndexNow: %v", err)
	}

	got, err := svc.Search("crumble", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Search crumble: %d résultats", len(got))
	}
}

func TestServiceAtomicWrite(t *testing.T) {
	svc, dir := setupVault(t)
	note, err := svc.CreateNote("Atomic", "")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	note.Content = "premier contenu"
	if _, err := svc.SaveNote(note); err != nil {
		t.Fatalf("SaveNote 1: %v", err)
	}
	// Aucun fichier .tmp ne doit subsister.
	matches, err := filepath.Glob(filepath.Join(dir, "notes", "**", "*.tmp-*"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("fichiers temporaires résiduels : %v", matches)
	}
}

func TestServiceRestoreConflict(t *testing.T) {
	svc, _ := setupVault(t)
	note, err := svc.CreateNote("Conflict", "")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if err := svc.DeleteNote(note.RelativePath); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	trash, _ := svc.ListTrash()
	if len(trash) != 1 {
		t.Fatalf("corbeille: %d", len(trash))
	}
	// On recrée une note qui va écraser le chemin original (date arrondie à la seconde).
	note2, err := svc.CreateNote("Conflict", "")
	if err != nil {
		t.Fatalf("CreateNote 2: %v", err)
	}
	if note2.RelativePath != note.RelativePath {
		// Si le timestamp change, on force le conflit en écrivant le même chemin.
		extra, err := svc.CreateNote("Conflict", "")
		if err != nil {
			t.Fatalf("CreateNote 3: %v", err)
		}
		// Écraser le chemin original avec un fichier vide.
		_ = os.WriteFile(filepath.Join(svc.Root(), note.RelativePath), []byte("---\n---\n"), 0o644)
		_ = extra
	}
	if _, err := svc.RestoreFromTrash(trash[0].ID); err == nil {
		t.Fatal("RestoreFromTrash aurait dû échouer (conflit)")
	}
}

func TestServiceConfig(t *testing.T) {
	svc, dir := setupVault(t)
	cfg, err := svc.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if cfg.Theme != "dark" {
		t.Fatalf("Theme par défaut: %s", cfg.Theme)
	}
	cfg.Theme = "light"
	cfg.AutoDailyNote = true
	if err := svc.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	cfg2, _ := svc.GetConfig()
	if cfg2.Theme != "light" || !cfg2.AutoDailyNote {
		t.Fatalf("config non persistée : %+v", cfg2)
	}
	configPath := filepath.Join(dir, ".notevault", "config.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config.json introuvable : %v", err)
	}
}

func TestServicePurgeTrash(t *testing.T) {
	svc, dir := setupVault(t)
	note, _ := svc.CreateNote("Old", "")
	if err := svc.DeleteNote(note.RelativePath); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	// Forcer une date ancienne dans le sidecar.
	trash, err := svc.ListTrash()
	if err != nil || len(trash) != 1 {
		t.Fatalf("ListTrash: %v / %d", err, len(trash))
	}
	metaPath := trash[0].TrashPath + ".meta"
	old := time.Now().AddDate(-1, 0, 0).UTC().Format(time.RFC3339)
	if err := os.WriteFile(metaPath, []byte("original: "+note.RelativePath+"\ntrashed_at: "+old+"\n"), 0o644); err != nil {
		t.Fatalf("rewrite meta: %v", err)
	}
	// Purge avec retention = 1 jour.
	if err := purgeTrash(dir, 1); err != nil {
		t.Fatalf("purgeTrash: %v", err)
	}
	trashAfter, _ := svc.ListTrash()
	if len(trashAfter) != 0 {
		t.Fatalf("corbeille non vidée : %d", len(trashAfter))
	}
}

func TestServiceCreateNoteTemplates(t *testing.T) {
	svc, _ := setupVault(t)
	cases := []struct{ key, want string }{
		{"meeting", "# Participants"},
		{"daily", "# Intention"},
		{"unknown", ""},
	}
	for _, c := range cases {
		note, err := svc.CreateNote("Test "+c.key, c.key)
		if err != nil {
			t.Fatalf("CreateNote %s: %v", c.key, err)
		}
		if !contains(note.Content, c.want) {
			t.Fatalf("template %s: manque %q dans %q", c.key, c.want, note.Content)
		}
	}
}

func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
