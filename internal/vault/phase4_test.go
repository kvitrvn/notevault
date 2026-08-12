package vault

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServiceSaveAsset(t *testing.T) {
	svc, dir := setupVault(t)
	data := []byte("fake-png-bytes")
	rel, err := svc.SaveAsset(data, "photo.png")
	if err != nil {
		t.Fatalf("SaveAsset: %v", err)
	}
	if !strings.HasPrefix(rel, "assets/") {
		t.Fatalf("rel: %s", rel)
	}
	if !strings.HasSuffix(rel, ".png") {
		t.Fatalf("extension: %s", rel)
	}
	abs := filepath.Join(dir, rel)
	got, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(data) {
		t.Fatal("contenu différent")
	}
}

func TestServiceSaveAssetRejectsBadExt(t *testing.T) {
	svc, _ := setupVault(t)
	_, err := svc.SaveAsset([]byte("boom"), "malware.exe")
	if err == nil {
		t.Fatal("aurait dû rejeter l'extension")
	}
}

func TestServiceSaveAssetMaxSize(t *testing.T) {
	svc, _ := setupVault(t)
	data := make([]byte, 1000)
	_, err := svc.SaveAssetWithMaxSize(data, "fichier.txt", 100)
	if err == nil {
		t.Fatal("aurait dû rejeter un fichier trop gros")
	}
}

func TestServiceGetBacklinks(t *testing.T) {
	svc, _ := setupVault(t)
	// Crée une note cible.
	target, _ := svc.CreateNote("", "Projets Q3", "")
	target.Content = "Auto-référence : [[Projets Q3]]."
	if _, err := svc.SaveNote(target); err != nil {
		t.Fatalf("SaveNote target: %v", err)
	}
	// Crée une note qui la lie explicitement.
	a, _ := svc.CreateNote("", "Suivi", "")
	a.Content = "Voir [[Projets Q3]] pour la liste."
	if _, err := svc.SaveNote(a); err != nil {
		t.Fatalf("SaveNote a: %v", err)
	}
	// Une simple mention du titre ne constitue pas un backlink.
	b, _ := svc.CreateNote("", "Retro", "")
	b.Content = "Le document Projets Q3 est prêt."
	if _, err := svc.SaveNote(b); err != nil {
		t.Fatalf("SaveNote b: %v", err)
	}
	// Indexation.
	if err := svc.IndexNow(context.Background(), nil); err != nil {
		t.Fatalf("IndexNow: %v", err)
	}
	// Backlinks de "Projets Q3" → la note "Suivi" (la cible exclue).
	links, err := svc.GetBacklinks("Projets Q3", target.RelativePath, 10)
	if err != nil {
		t.Fatalf("GetBacklinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("backlinks: %d (%+v)", len(links), links)
	}
	if links[0].RelativePath != a.RelativePath {
		t.Fatalf("backlink pointe vers %s (attendu %s)", links[0].RelativePath, a.RelativePath)
	}
	for _, link := range links {
		if link.RelativePath == b.RelativePath {
			t.Fatal("une mention en texte libre ne doit pas créer de backlink")
		}
	}
	// Sans exclusion, la cible apparaît aussi (vérification du paramètre).
	allLinks, _ := svc.GetBacklinks("Projets Q3", "", 10)
	if len(allLinks) != 2 {
		t.Fatalf("sans exclusion: %d", len(allLinks))
	}
}

func TestServiceHistoryBasic(t *testing.T) {
	svc, dir := setupVault(t)
	note, _ := svc.CreateNote("", "Versionnée", "")
	for i := 0; i < 3; i++ {
		note.Content = "v" + string(rune('1'+i))
		if _, err := svc.SaveNote(note); err != nil {
			t.Fatalf("SaveNote %d: %v", i, err)
		}
	}
	hist, err := svc.ListHistory(note.RelativePath)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(hist) != 3 {
		t.Fatalf("history: %d versions (%+v)", len(hist), hist)
	}
	// Vérifie l'ordre : plus récent d'abord.
	for i := 0; i < len(hist)-1; i++ {
		if hist[i].ID < hist[i+1].ID {
			t.Fatal("ordre incorrect")
		}
	}
	// Restaure la version qui contient "v1" (hist[1]).
	var target *HistoryEntry
	for i, h := range hist {
		if strings.Contains(h.Preview, "v1") {
			target = &hist[i]
			break
		}
	}
	if target == nil {
		t.Fatal("version v1 introuvable dans l'historique")
	}
	restored, err := svc.RestoreFromHistory(note.RelativePath, target.ID)
	if err != nil {
		t.Fatalf("RestoreFromHistory: %v", err)
	}
	if !strings.Contains(restored.Content, "v1") {
		t.Fatalf("restauré: %q", restored.Content)
	}
	// L'historique doit contenir la version courante (devenue une nouvelle
	// entrée après le restore).
	hist2, _ := svc.ListHistory(note.RelativePath)
	if len(hist2) != 4 {
		t.Fatalf("après restore: %d", len(hist2))
	}
	// Sanity : fichier existe bien.
	if _, err := os.Stat(filepath.Join(dir, note.RelativePath)); err != nil {
		t.Fatalf("fichier absent après restore : %v", err)
	}
}

func TestServiceHistoryRotation(t *testing.T) {
	svc, _ := setupVault(t)
	cfg, _ := svc.GetConfig()
	cfg.HistoryPerNote = 2
	if err := svc.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	note, _ := svc.CreateNote("", "Tourne", "")
	for i := 0; i < 5; i++ {
		note.Content = "v" + string(rune('1'+i))
		_, _ = svc.SaveNote(note)
		time.Sleep(2 * time.Millisecond) // timestamp différent
	}
	hist, _ := svc.ListHistory(note.RelativePath)
	if len(hist) != 2 {
		t.Fatalf("rotation: %d versions (max 2 attendues)", len(hist))
	}
}

func TestServiceDiffHistory(t *testing.T) {
	svc, _ := setupVault(t)
	note, _ := svc.CreateNote("", "Diff", "")
	note.Content = "ligne A\nligne B\nligne C\n"
	v1, _ := svc.SaveNote(note)
	// Modifie et sauve deux fois pour obtenir deux snapshots.
	note.Content = "ligne A\nligne B modifiée\nligne C\n"
	_, _ = svc.SaveNote(note)
	note.Content = "ligne A\nligne B modifiée\nligne C\nligne D\n"
	_, _ = svc.SaveNote(note)
	hist, _ := svc.ListHistory(v1.RelativePath)
	if len(hist) < 2 {
		t.Fatalf("history: %d", len(hist))
	}
	// hist[0] = snapshot le plus récent (= v1 modifié) ; hist[1] = v1 original.
	diff, err := svc.DiffHistory(v1.RelativePath, hist[1].ID, hist[0].ID)
	if err != nil {
		t.Fatalf("DiffHistory: %v", err)
	}
	if !strings.Contains(diff, "modifiée") {
		t.Fatalf("diff inattendu : %q", diff)
	}
	if !strings.Contains(diff, "@@") {
		t.Fatalf("pas de hunk : %q", diff)
	}
}

func TestServiceReadHistoryVersion(t *testing.T) {
	svc, _ := setupVault(t)
	note, _ := svc.CreateNote("", "Read", "")
	note.Content = "version 1"
	_, _ = svc.SaveNote(note)
	note.Content = "version 2"
	_, _ = svc.SaveNote(note)
	hist, _ := svc.ListHistory(note.RelativePath)
	// hist[0] est le snapshot le plus récent (avant "version 2") qui
	// contient donc "version 1".
	raw, err := svc.ReadHistoryVersion(note.RelativePath, hist[0].ID)
	if err != nil {
		t.Fatalf("ReadHistoryVersion: %v", err)
	}
	if !strings.Contains(raw, "version 1") {
		t.Fatalf("contenu : %q", raw)
	}
}

func TestUnifiedDiffEmpty(t *testing.T) {
	diff := unifiedDiff("a", "abc", "b", "abd")
	if !strings.Contains(diff, "-abc") || !strings.Contains(diff, "+abd") {
		t.Fatalf("diff inattendu : %q", diff)
	}
}
