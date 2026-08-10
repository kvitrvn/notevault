package vault

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/kvitrvn/notevault/internal/domain"
)

// captureHandler accumule les batches publiés par le changeBus.
type captureHandler struct {
	mu      sync.Mutex
	batches []VaultChangeBatch
}

func (c *captureHandler) Callback(b VaultChangeBatch) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.batches = append(c.batches, b)
}

func (c *captureHandler) Changes() []VaultChange {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]VaultChange, 0)
	for _, b := range c.batches {
		out = append(out, b.Changes...)
	}
	return out
}

func (c *captureHandler) HasFullRescan() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, b := range c.batches {
		if b.FullRescan {
			return true
		}
	}
	return false
}

func TestApplyFsEvents_UpsertMarkdown(t *testing.T) {
	svc, dir := setupVault(t)
	capture := &captureHandler{}
	svc.OnChange(capture.Callback)

	target := filepath.Join(dir, "notes", "inbox", "alpha.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("---\ntitle: Alpha\n---\n\nbody"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	changes, fullRescan, err := svc.ApplyFsEvents([]fsObservation{
		{path: target, kind: fsEventUpsert},
	})
	if err != nil {
		t.Fatalf("ApplyFsEvents: %v", err)
	}
	if fullRescan {
		t.Fatalf("attendu !fullRescan, obtenu fullRescan")
	}
	if len(changes) != 1 || changes[0].Kind != ChangeUpsert || changes[0].Path != "notes/inbox/alpha.md" {
		t.Fatalf("changes: %#v", changes)
	}
	if _, err := svc.index.Get("notes/inbox/alpha.md"); err != nil {
		t.Fatalf("index.Get: %v", err)
	}

	// Le batch doit avoir été publié via le bus.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(capture.Changes()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(capture.Changes()) == 0 {
		t.Fatalf("aucun batch publié")
	}
}

func TestApplyFsEvents_RemoveNonMarkdownIgnores(t *testing.T) {
	svc, dir := setupVault(t)
	target := filepath.Join(dir, "notes", "inbox", "image.png")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("png"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	changes, _, err := svc.ApplyFsEvents([]fsObservation{
		{path: target, kind: fsEventUpsert},
		{path: target, kind: fsEventRemove},
	})
	if err != nil {
		t.Fatalf("ApplyFsEvents: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("changes attendus=0, obtenus=%#v", changes)
	}
}

func TestApplyFsEvents_FolderUpsertScansExistingFiles(t *testing.T) {
	svc, dir := setupVault(t)
	folder := filepath.Join(dir, "notes", "project")
	if err := os.MkdirAll(filepath.Join(folder, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "a.md"), []byte("# A"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "sub", "b.md"), []byte("# B"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	changes, _, err := svc.ApplyFsEvents([]fsObservation{
		{path: folder, kind: fsEventUpsert, isDir: true},
	})
	if err != nil {
		t.Fatalf("ApplyFsEvents: %v", err)
	}
	got := changePaths(changes)
	want := []string{"notes/project/a.md", "notes/project/sub/b.md"}
	if !sameStringSet(got, want) {
		t.Fatalf("changements: %#v attendus %#v", got, want)
	}
	if _, err := svc.index.Get("notes/project/sub/b.md"); err != nil {
		t.Fatalf("index.Get b.md: %v", err)
	}
}

func TestApplyFsEvents_FolderRemovePurgesSubtree(t *testing.T) {
	svc, dir := setupVault(t)
	folder := filepath.Join(dir, "notes", "project")
	if err := os.MkdirAll(filepath.Join(folder, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, name := range []string{"a.md", "sub/b.md"} {
		if err := svc.index.Upsert(domain.Note{
			RelativePath: "notes/project/" + name,
			Title:        name,
			Content:      "",
			UpdatedAt:    time.Now().UTC(),
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	changes, _, err := svc.ApplyFsEvents([]fsObservation{
		{path: folder, kind: fsEventRemove, isDir: true},
	})
	if err != nil {
		t.Fatalf("ApplyFsEvents: %v", err)
	}
	got := changePaths(changes)
	want := []string{"notes/project/a.md", "notes/project/sub/b.md"}
	if !sameStringSet(got, want) {
		t.Fatalf("changements: %#v attendus %#v", got, want)
	}
	if _, err := svc.index.Get("notes/project/a.md"); err == nil {
		t.Fatalf("a.md aurait dû être purgé")
	}
	if _, err := svc.index.Get("notes/project/sub/b.md"); err == nil {
		t.Fatalf("sub/b.md aurait dû être purgé")
	}
}

func TestApplyFsEvents_RescanSentinel(t *testing.T) {
	svc, dir := setupVault(t)
	capture := &captureHandler{}
	svc.OnChange(capture.Callback)

	// La note pré-existe (sans index) ; le rescan doit la découvrir.
	target := filepath.Join(dir, "notes", "inbox", "external.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("# External\n\nbody"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	changes, fullRescan, err := svc.ApplyFsEvents([]fsObservation{
		{path: fsRescanSentinel, kind: fsEventUpsert},
	})
	if err != nil {
		t.Fatalf("ApplyFsEvents: %v", err)
	}
	if !fullRescan {
		t.Fatalf("fullRescan attendu")
	}
	got := changePaths(changes)
	if !containsString(got, "notes/inbox/external.md") {
		t.Fatalf("external.md absent: %#v", got)
	}
	if !capture.HasFullRescan() {
		t.Fatalf("batch FullRescan non publié")
	}
	if _, err := svc.index.Get("notes/inbox/external.md"); err != nil {
		t.Fatalf("external.md absent de l'index après rescan")
	}
}

func TestApplyFsEvents_OutsideNotesRemovesIndexEntry(t *testing.T) {
	svc, dir := setupVault(t)
	// Seed : une entrée d'index hors zone notes/ (cas théorique, smoke test).
	if err := svc.index.Upsert(domain.Note{
		RelativePath: "templates/legacy.md",
		Title:        "Legacy",
		UpdatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	target := filepath.Join(dir, "templates", "legacy.md")
	changes, _, err := svc.ApplyFsEvents([]fsObservation{
		{path: target, kind: fsEventRemove},
	})
	if err != nil {
		t.Fatalf("ApplyFsEvents: %v", err)
	}
	if len(changes) != 1 || changes[0].Path != "templates/legacy.md" || changes[0].Kind != ChangeDelete {
		t.Fatalf("changes: %#v", changes)
	}
}

func TestWatcherCoalescesBurst(t *testing.T) {
	svc, dir := setupVault(t)
	notesRoot := filepath.Join(dir, "notes", "inbox")
	if err := os.MkdirAll(notesRoot, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Démarre le watcher APRÈS avoir créé le dossier racine.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := NewWatcher(ctx, svc.root, svc); err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	// On ne peut pas Close() via la session ici ; on annulera via cancel.

	// Écriture en rafale de plusieurs fichiers.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			path := filepath.Join(notesRoot, "burst-"+itoa(i)+".md")
			_ = os.WriteFile(path, []byte("# "+itoa(i)), 0o644)
		}
		close(done)
	}()
	<-done

	// Attends que l'index soit cohérent.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		notes, err := svc.ListNotesFiltered(FilterQuery{}, 100)
		if err == nil && len(notes) >= 10 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	notes, _ := svc.ListNotesFiltered(FilterQuery{}, 100)
	t.Fatalf("burst non indexé : %d notes (attendu >= 10)", len(notes))
}

func TestWatcherRecursesIntoCopiedFolder(t *testing.T) {
	svc, dir := setupVault(t)
	notesRoot := filepath.Join(dir, "notes")
	src := filepath.Join(dir, "staging", "pasted")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.md"), []byte("# A"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "b.md"), []byte("# B"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w, err := NewWatcher(ctx, svc.root, svc)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	// Simule un déplacement atomique d'une arborescence déjà remplie.
	if err := os.Rename(src, filepath.Join(notesRoot, "pasted")); err != nil {
		t.Fatalf("rename: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	want := []string{"notes/pasted/a.md", "notes/pasted/sub/b.md"}
	for time.Now().Before(deadline) {
		notes, err := svc.ListNotesFiltered(FilterQuery{}, 100)
		if err != nil {
			break
		}
		paths := make([]string, 0)
		for _, n := range notes {
			paths = append(paths, n.RelativePath)
		}
		if containsString(paths, "notes/pasted/a.md") && containsString(paths, "notes/pasted/sub/b.md") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("dossier collé non indexé: attendus %#v", want)
}

func changePaths(changes []VaultChange) []string {
	out := make([]string, 0, len(changes))
	for _, c := range changes {
		out = append(out, c.Path)
	}
	sort.Strings(out)
	return out
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}

func containsString(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}
