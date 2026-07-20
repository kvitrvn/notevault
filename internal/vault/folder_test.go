package vault

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceCreateFolderRoot(t *testing.T) {
	svc, dir := setupVault(t)
	if _, err := svc.CreateFolder("", "Projets"); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes", "projets")); err != nil {
		t.Fatalf("dossier attendu sur disque : %v", err)
	}
	folders, err := svc.ListFolders()
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	found := false
	for _, f := range folders {
		if f.Path == "projets" {
			found = true
			if f.Count != 0 {
				t.Fatalf("Count dossier vide: %d", f.Count)
			}
		}
	}
	if !found {
		t.Fatalf("dossier projets manquant : %+v", folders)
	}
}

func TestServiceCreateFolderNested(t *testing.T) {
	svc, dir := setupVault(t)
	if _, err := svc.CreateFolder("", "Projets"); err != nil {
		t.Fatalf("CreateFolder racine: %v", err)
	}
	if _, err := svc.CreateFolder("notes/projets", "Web"); err != nil {
		t.Fatalf("CreateFolder imbriqué: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes", "projets", "web")); err != nil {
		t.Fatalf("dossier imbriqué attendu : %v", err)
	}
}

func TestServiceCreateFolderRejectsParentOutsideNotes(t *testing.T) {
	svc, _ := setupVault(t)
	if _, err := svc.CreateFolder("assets", "x"); err == nil {
		t.Fatal("CreateFolder dans assets/ aurait dû échouer")
	}
	if _, err := svc.CreateFolder("..", "x"); err == nil {
		t.Fatal("CreateFolder avec parent .. aurait dû échouer")
	}
}

func TestServiceCreateFolderSlugNormalizesName(t *testing.T) {
	svc, dir := setupVault(t)
	if _, err := svc.CreateFolder("", "Mes Projets 2026!"); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes", "mes-projets-2026")); err != nil {
		t.Fatalf("slug attendu : %v", err)
	}
}

func TestServiceCreateFolderRejectsTraversal(t *testing.T) {
	svc, _ := setupVault(t)
	cases := []string{"../escape", "/abs/path", "..", "foo/../bar"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := svc.CreateFolder("", name)
			if err == nil {
				t.Fatalf("CreateFolder(%q) aurait dû échouer", name)
			}
		})
	}
}

func TestServiceCreateFolderRejectsBlankName(t *testing.T) {
	svc, _ := setupVault(t)
	for _, name := range []string{"", "   ", "!!!"} {
		if _, err := svc.CreateFolder("", name); err == nil {
			t.Fatalf("CreateFolder(%q) aurait dû échouer", name)
		}
	}
}

func TestServiceCreateFolderRejectsExisting(t *testing.T) {
	svc, _ := setupVault(t)
	if _, err := svc.CreateFolder("", "Projets"); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	_, err := svc.CreateFolder("", "Projets")
	if !errors.Is(err, ErrFolderExists) {
		t.Fatalf("erreur attendue ErrFolderExists, reçu : %v", err)
	}
}

func TestServiceCreateNoteInFolder(t *testing.T) {
	svc, _ := setupVault(t)
	if _, err := svc.CreateFolder("", "Projets"); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	note, err := svc.CreateNote("notes/projets", "Bienvenue", "blank")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if !strings.HasPrefix(note.RelativePath, "notes/projets/") {
		t.Fatalf("chemin inattendu : %s", note.RelativePath)
	}
	if err := svc.IndexNow(context.Background(), nil); err != nil {
		t.Fatalf("IndexNow: %v", err)
	}
	folders, err := svc.ListFolders()
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	var projets FolderInfo
	for _, f := range folders {
		if f.Path == "projets" {
			projets = f
		}
	}
	if projets.Path == "" {
		t.Fatalf("dossier projets introuvable : %+v", folders)
	}
	if projets.Count != 1 {
		t.Fatalf("Count pour projets : %d (attendu 1)", projets.Count)
	}
}

func TestServiceCreateNoteDefaultsToInbox(t *testing.T) {
	svc, _ := setupVault(t)
	note, err := svc.CreateNote("", "Legacy", "")
	if err != nil {
		t.Fatalf("CreateNote legacy: %v", err)
	}
	if !strings.HasPrefix(note.RelativePath, "notes/inbox/") {
		t.Fatalf("chemin par défaut cassé : %s", note.RelativePath)
	}
}

func TestServiceCreateNoteInNestedFolder(t *testing.T) {
	svc, _ := setupVault(t)
	if _, err := svc.CreateFolder("", "Projets"); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if _, err := svc.CreateFolder("notes/projets", "Web"); err != nil {
		t.Fatalf("CreateFolder nested: %v", err)
	}
	note, err := svc.CreateNote("notes/projets/web", "Index", "")
	if err != nil {
		t.Fatalf("CreateNote nested: %v", err)
	}
	if !strings.HasPrefix(note.RelativePath, "notes/projets/web/") {
		t.Fatalf("chemin inattendu : %s", note.RelativePath)
	}
}

func TestServiceListFoldersPicksEmptyDirCreatedOnDisk(t *testing.T) {
	svc, dir := setupVault(t)
	if err := os.MkdirAll(filepath.Join(dir, "notes", "vide"), 0o755); err != nil {
		t.Fatalf("mkdir manuel : %v", err)
	}
	folders, err := svc.ListFolders()
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	found := false
	for _, f := range folders {
		if f.Path == "vide" {
			found = true
			if f.Count != 0 {
				t.Fatalf("Count: %d", f.Count)
			}
		}
	}
	if !found {
		t.Fatalf("dossier vide non listé : %+v", folders)
	}
}
