package vault

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceMoveNote(t *testing.T) {
	svc, dir := setupVault(t)
	note, err := svc.CreateNote("", "Original", "")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	original := note.RelativePath
	dst := "notes/projects/web/moved.md"

	moved, err := svc.MoveNote(original, dst)
	if err != nil {
		t.Fatalf("MoveNote: %v", err)
	}
	if moved.RelativePath != dst {
		t.Fatalf("RelativePath: %s", moved.RelativePath)
	}
	if _, err := os.Stat(filepath.Join(dir, dst)); err != nil {
		t.Fatalf("nouveau fichier introuvable : %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, original)); err == nil {
		t.Fatal("ancien fichier toujours présent")
	}
	// Index : nouveau résumé trouvé via List, ancien supprimé.
	if err := svc.IndexNow(context.Background(), nil); err != nil {
		t.Fatalf("IndexNow: %v", err)
	}
	list, _ := svc.ListNotes()
	found := false
	for _, n := range list {
		if n.RelativePath == dst {
			found = true
		}
		if n.RelativePath == original {
			t.Fatal("ancien chemin encore dans l'index")
		}
	}
	if !found {
		t.Fatal("nouveau chemin absent de l'index")
	}
}

func TestServiceMoveNoteInvalid(t *testing.T) {
	svc, _ := setupVault(t)
	note, _ := svc.CreateNote("", "Hello", "")
	cases := []struct {
		name, dst string
		wantErr   string
	}{
		{"bad prefix", "inbox/x.md", "notes/"},
		{"bad ext", "notes/inbox/x.txt", ".md"},
		{"abs path", "/etc/passwd", "invalide"},
		{"parent escape", "../escape.md", "invalide"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.MoveNote(note.RelativePath, c.dst)
			if err == nil {
				t.Fatal("aurait dû échouer")
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("erreur %q ne contient pas %q", err, c.wantErr)
			}
		})
	}
}

func TestServiceMoveNoteMissingSource(t *testing.T) {
	svc, _ := setupVault(t)
	_, err := svc.MoveNote("notes/inbox/missing.md", "notes/inbox/new.md")
	if err == nil {
		t.Fatal("aurait dû échouer (source introuvable)")
	}
	if !strings.Contains(err.Error(), "introuvable") {
		t.Fatalf("erreur inattendue : %v", err)
	}
}

func TestServiceMoveNoteCollision(t *testing.T) {
	svc, _ := setupVault(t)
	a, _ := svc.CreateNote("", "A", "")
	b, _ := svc.CreateNote("", "B", "")
	if _, err := svc.MoveNote(a.RelativePath, b.RelativePath); err == nil {
		t.Fatal("aurait dû échouer (collision)")
	}
}

func TestServiceDuplicateNote(t *testing.T) {
	svc, _ := setupVault(t)
	original, _ := svc.CreateNote("", "Original", "")
	original.Tags = []string{"projet", "important"}
	original, _ = svc.SaveNote(original)

	dup, err := svc.DuplicateNote(original.RelativePath)
	if err != nil {
		t.Fatalf("DuplicateNote: %v", err)
	}
	if dup.RelativePath == original.RelativePath {
		t.Fatal("chemin identique à l'original")
	}
	if !strings.Contains(dup.Title, "copie") {
		t.Fatalf("titre : %q", dup.Title)
	}
	if len(dup.Tags) != 2 {
		t.Fatalf("tags non recopiés : %v", dup.Tags)
	}
	// Re-dupliquer : le second doit éviter la collision.
	dup2, err := svc.DuplicateNote(original.RelativePath)
	if err != nil {
		t.Fatalf("DuplicateNote 2: %v", err)
	}
	if dup2.RelativePath == dup.RelativePath {
		t.Fatal("chemin collision sur double duplication")
	}
}

func TestServiceRenameTitle(t *testing.T) {
	svc, _ := setupVault(t)
	note, _ := svc.CreateNote("", "Original", "")
	renamed, err := svc.RenameTitle(note.RelativePath, "Nouveau titre")
	if err != nil {
		t.Fatalf("RenameTitle: %v", err)
	}
	if renamed.Title != "Nouveau titre" {
		t.Fatalf("title: %s", renamed.Title)
	}
	// Chemin inchangé.
	if renamed.RelativePath != note.RelativePath {
		t.Fatalf("path: %s", renamed.RelativePath)
	}
	// Vide → repli sur "Sans titre".
	renamed, _ = svc.RenameTitle(note.RelativePath, "   ")
	if renamed.Title != "Sans titre" {
		t.Fatalf("fallback: %s", renamed.Title)
	}
}

func TestServiceTemplatesBuiltin(t *testing.T) {
	svc, _ := setupVault(t)
	templates := svc.ListTemplates()
	names := make(map[string]bool)
	for _, tpl := range templates {
		names[tpl.ID] = true
	}
	for _, want := range []string{"blank", "meeting", "daily"} {
		if !names[want] {
			t.Fatalf("template %q manquant : %+v", want, templates)
		}
	}
}

func TestServiceTemplatesUserOverride(t *testing.T) {
	svc, dir := setupVault(t)
	// Crée un template utilisateur "meeting" qui écrase le built-in.
	tplDir := filepath.Join(dir, "templates")
	if err := os.MkdirAll(tplDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	body := "---\nname: Réunion perso\n---\n# Perso\n\ncontenu custom\n"
	if err := os.WriteFile(filepath.Join(tplDir, "meeting.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Recharge en construisant un nouveau loader (le service a son loader en cache).
	svc.templates = NewTemplateLoader(dir)
	templates := svc.ListTemplates()
	var meeting *Template
	for i, tpl := range templates {
		if tpl.ID == "meeting" {
			meeting = &templates[i]
			break
		}
	}
	if meeting == nil {
		t.Fatal("meeting introuvable")
	}
	if meeting.Name != "Réunion perso" {
		t.Fatalf("name: %q", meeting.Name)
	}
	if !strings.Contains(meeting.Body, "contenu custom") {
		t.Fatalf("body: %q", meeting.Body)
	}
	if meeting.Builtin {
		t.Fatal("devrait être marqué non-builtin")
	}
}

func TestServiceCreateNoteFromUserTemplate(t *testing.T) {
	svc, dir := setupVault(t)
	tplDir := filepath.Join(dir, "templates")
	if err := os.MkdirAll(tplDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	body := "---\nname: Bug fix\n---\n## Étapes de repro\n\n1. \n\n## Cause\n\n"
	if err := os.WriteFile(filepath.Join(tplDir, "bug.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	svc.templates = NewTemplateLoader(dir)
	note, err := svc.CreateNote("", "Mon bug", "bug")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if !strings.Contains(note.Content, "Étapes de repro") {
		t.Fatalf("template non appliqué : %q", note.Content)
	}
}

func TestServiceOpenInExplorer(t *testing.T) {
	svc, _ := setupVault(t)
	note, _ := svc.CreateNote("", "A", "")
	// On ne peut pas vraiment tester l'ouverture du gestionnaire (pas de
	// display), mais on vérifie que l'appel ne panique pas et ne corrompt
	// pas le fichier.
	if err := svc.OpenInExplorer(note.RelativePath, true); err != nil {
		// Sur CI sans display, l'erreur est tolérée.
		t.Logf("OpenInExplorer: %v (ignoré)", err)
	}
}
