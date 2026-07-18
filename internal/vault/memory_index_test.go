package vault

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/kvitrvn/notevault/internal/domain"
)

func TestMemoryIndexSearchFiltersAndPins(t *testing.T) {
	root := t.TempDir()
	indexValue, err := newMemoryIndex(root)
	if err != nil {
		t.Fatalf("newMemoryIndex: %v", err)
	}
	idx := indexValue.(*memoryIndex)
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	notes := []domain.Note{
		{RelativePath: "notes/projets/alpha.md", Title: "Café Alpha", Content: "la phrase exacte apparaît ici", Tags: []string{"projet", "important"}, CreatedAt: now, UpdatedAt: now},
		{RelativePath: "notes/projets/beta.md", Title: "Beta", Content: "phrase séparée mais pas exacte", Tags: []string{"projet", "archive"}, CreatedAt: now, UpdatedAt: now.Add(time.Hour)},
		{RelativePath: "notes/perso/gamma.md", Title: "Gamma", Content: "café", Tags: []string{"perso"}, CreatedAt: now, UpdatedAt: now.Add(2 * time.Hour)},
	}
	for _, note := range notes {
		if err := idx.Upsert(note); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	results, err := idx.Search(`"phrase exacte"`, SearchOpts{Limit: 10})
	if err != nil || len(results) != 1 || results[0].RelativePath != notes[0].RelativePath {
		t.Fatalf("phrase search = %+v, %v", results, err)
	}
	results, err = idx.List(ListFilter{Folder: "projets", Tags: []string{"projet"}, ExcludeTags: []string{"archive"}, Query: "cafe", Limit: 10})
	if err != nil || len(results) != 1 || results[0].RelativePath != notes[0].RelativePath {
		t.Fatalf("combined filters = %+v, %v", results, err)
	}
	if err := idx.Pin(notes[0].RelativePath, true); err != nil {
		t.Fatalf("Pin: %v", err)
	}

	reloadedValue, err := newMemoryIndex(root)
	if err != nil {
		t.Fatalf("reload index: %v", err)
	}
	reloaded := reloadedValue.(*memoryIndex)
	if err := reloaded.Upsert(notes[0]); err != nil {
		t.Fatalf("reload Upsert: %v", err)
	}
	pinned, err := reloaded.ListPinned()
	if err != nil || len(pinned) != 1 || pinned[0].RelativePath != notes[0].RelativePath {
		t.Fatalf("persisted pins = %+v, %v", pinned, err)
	}
}

func TestMemoryIndexSummariesIncludeIndependentTags(t *testing.T) {
	t.Parallel()
	indexValue, err := newMemoryIndex(t.TempDir())
	if err != nil {
		t.Fatalf("newMemoryIndex: %v", err)
	}
	idx := indexValue.(*memoryIndex)
	note := domain.Note{RelativePath: "notes/a.md", Title: "A", Tags: []string{"dpo", "rd"}}
	if err := idx.Upsert(note); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	summaries, err := idx.List(ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(summaries) != 1 || !slices.Equal(summaries[0].Tags, note.Tags) {
		t.Fatalf("tags du résumé = %v, want %v", summaries, note.Tags)
	}
	summaries[0].Tags[0] = "modifie"
	stored, err := idx.Get(note.RelativePath)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Tags[0] != "dpo" {
		t.Fatalf("la mutation du résumé a modifié l’index : %v", stored.Tags)
	}
}

func TestMemoryIndexConcurrentAccess(t *testing.T) {
	t.Parallel()
	indexValue, err := newMemoryIndex(t.TempDir())
	if err != nil {
		t.Fatalf("newMemoryIndex: %v", err)
	}
	idx := indexValue.(*memoryIndex)
	var group sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			for n := 0; n < 100; n++ {
				path := fmt.Sprintf("notes/%d/%d.md", worker, n)
				_ = idx.Upsert(domain.Note{RelativePath: path, Title: "Concurrent", Content: "token", UpdatedAt: time.Now()})
				_, _ = idx.Search("token", SearchOpts{Limit: 25})
				_, _ = idx.ListTags()
			}
		}(worker)
	}
	group.Wait()
	paths, err := idx.ListPaths()
	if err != nil || len(paths) != 800 {
		t.Fatalf("ListPaths count = %d, %v", len(paths), err)
	}
}
