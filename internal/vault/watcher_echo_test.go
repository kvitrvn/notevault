package vault

import (
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kvitrvn/notevault/internal/domain"
)

// countingIndex wraps an Index pour compter les appels à Upsert et Delete.
// Permet de vérifier que le skip d'écho du watcher évite bien le second
// Upsert lors d'une écriture interne (cf. audit perf 2.1).
type countingIndex struct {
	Index
	upserts atomic.Int64
	deletes atomic.Int64
}

func (c *countingIndex) Upsert(note domain.Note) error {
	c.upserts.Add(1)
	return c.Index.Upsert(note)
}

func (c *countingIndex) Delete(rel string) error {
	c.deletes.Add(1)
	return c.Index.Delete(rel)
}

func TestReindexFromPathSkipsInternalEcho(t *testing.T) {
	svc, dir := setupVault(t)

	note, err := svc.CreateNote("Echo", "")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	absPath := filepath.Join(dir, filepath.FromSlash(note.RelativePath))

	initial, err := svc.index.Get(note.RelativePath)
	if err != nil {
		t.Fatalf("index.Get initial: %v", err)
	}

	// Simule l'écho que le watcher enverrait après un SaveNote : on
	// modifie le fichier sur disque sans toucher à l'index, puis on
	// enregistre une marque d'écriture interne (comme le ferait SaveNote).
	corrupt := []byte("---\ntitle: Echo\ncreated_at: 2026-01-01T10:00:00Z\nupdated_at: 2026-01-01T10:00:00Z\n---\n\nÉCHO QUI NE DOIT PAS ÊTRE INDEXÉ\n")
	if err := os.WriteFile(absPath, corrupt, 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	svc.markInternalWrite(absPath)

	svc.reindexFromPath(absPath)

	got, err := svc.index.Get(note.RelativePath)
	if err != nil {
		t.Fatalf("index.Get after: %v", err)
	}
	if got.Content != initial.Content {
		t.Fatalf("contenu indexé par l'écho, got=%q want=%q", got.Content, initial.Content)
	}
}

func TestReindexFromPathRunsForExternalChange(t *testing.T) {
	svc, dir := setupVault(t)

	// Fichier écrit hors du service : aucune marque d'écriture interne, le
	// reindex doit donc s'exécuter et propager le contenu dans l'index.
	relPath := "notes/inbox/extern.md"
	absPath := filepath.Join(dir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	newContent := "MODIFIÉ HORS APP\n"
	note := domain.Note{
		RelativePath: relPath,
		Title:        "External",
		Content:      newContent,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		Tags:         []string{},
	}
	if err := os.WriteFile(absPath, []byte(serialize(note)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	svc.reindexFromPath(absPath)

	got, err := svc.index.Get(relPath)
	if err != nil {
		t.Fatalf("index.Get after: %v", err)
	}
	if got.Content != newContent {
		t.Fatalf("modification externe non propagée, got=%q want=%q", got.Content, newContent)
	}
}

func TestConsumeInternalWriteExpiresAfterWindow(t *testing.T) {
	svc, _ := setupVault(t)
	svc.recentWriteMu.Lock()
	svc.recentWrites = map[string]time.Time{
		"notes/inbox/old.md": time.Now().Add(-2 * recentWriteWindow),
	}
	svc.recentWriteMu.Unlock()

	if _, ok := svc.consumeInternalWrite("notes/inbox/old.md"); ok {
		t.Fatal("une marque au-delà de recentWriteWindow aurait dû être expirée")
	}
}

func TestMarkInternalWritePrunesExpiredEntries(t *testing.T) {
	svc, _ := setupVault(t)
	svc.recentWriteMu.Lock()
	svc.recentWrites = make(map[string]time.Time)
	for i := 0; i < recentWriteCleanupThreshold+10; i++ {
		svc.recentWrites[filepath.Join("notes/inbox", "stale-"+strconv.Itoa(i)+".md")] =
			time.Now().Add(-2 * recentWriteWindow)
	}
	svc.recentWriteMu.Unlock()

	svc.markInternalWrite(filepath.Join(svc.root, "notes/inbox/fresh.md"))

	svc.recentWriteMu.Lock()
	defer svc.recentWriteMu.Unlock()
	for path := range svc.recentWrites {
		if path != filepath.Join(svc.root, "notes/inbox/fresh.md") {
			t.Fatalf("entrée expirée encore présente : %s", path)
		}
	}
}

func TestSaveNoteSuppressesWatcherEcho(t *testing.T) {
	dir := t.TempDir()
	baseIdx, err := newMemoryIndex(dir)
	if err != nil {
		t.Fatalf("newMemoryIndex: %v", err)
	}
	counted := &countingIndex{Index: baseIdx}

	svc, err := New(Options{Root: dir, StartWatcher: true, Index: counted})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	note, err := svc.CreateNote("Echo integ", "")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	// Laisse au watcher le temps de digérer l'écriture de CreateNote.
	time.Sleep(watcherDebounce + 300*time.Millisecond)

	baseline := counted.upserts.Load()

	note.Content = "deuxième contenu"
	if _, err := svc.SaveNote(note); err != nil {
		t.Fatalf("SaveNote: %v", err)
	}
	time.Sleep(watcherDebounce + 300*time.Millisecond)

	got := counted.upserts.Load() - baseline
	if want := int64(1); got != want {
		t.Fatalf("attendu %d upsert après SaveNote, observé %d (écho non supprimé)", want, got)
	}
}

func TestMoveNoteSuppressesWatcherEcho(t *testing.T) {
	dir := t.TempDir()
	baseIdx, err := newMemoryIndex(dir)
	if err != nil {
		t.Fatalf("newMemoryIndex: %v", err)
	}
	counted := &countingIndex{Index: baseIdx}

	svc, err := New(Options{Root: dir, StartWatcher: true, Index: counted})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	note, err := svc.CreateNote("Move echo", "")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	time.Sleep(watcherDebounce + 300*time.Millisecond)

	baseline := counted.upserts.Load()
	newRel := "notes/inbox/renamed-move.md"
	if _, err := svc.MoveNote(note.RelativePath, newRel); err != nil {
		t.Fatalf("MoveNote: %v", err)
	}
	time.Sleep(watcherDebounce + 300*time.Millisecond)

	got := counted.upserts.Load() - baseline
	if want := int64(1); got != want {
		t.Fatalf("attendu %d upsert après MoveNote, observé %d (écho non supprimé)", want, got)
	}
}
