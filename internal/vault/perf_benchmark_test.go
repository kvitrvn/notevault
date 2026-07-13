package vault

import (
	"fmt"
	"testing"
	"time"

	"github.com/kvitrvn/notevault/internal/domain"
)

func BenchmarkListNotes_10k(b *testing.B) {
	svc := benchmarkServiceWithNotes(b, 10_000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		notes, err := svc.ListNotes()
		if err != nil {
			b.Fatalf("ListNotes: %v", err)
		}
		if len(notes) == 0 {
			b.Fatal("ListNotes returned no notes")
		}
	}
}

func BenchmarkListFolders_10k(b *testing.B) {
	svc := benchmarkServiceWithNotes(b, 10_000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		folders, err := svc.ListFolders()
		if err != nil {
			b.Fatalf("ListFolders: %v", err)
		}
		if len(folders) == 0 {
			b.Fatal("ListFolders returned no folders")
		}
	}
}

func BenchmarkBuildIndex_10k(b *testing.B) {
	notes := benchmarkNotes(10_000)
	b.ResetTimer()
	for b.Loop() {
		idx := &memoryIndex{
			notes:    make(map[string]domain.Note),
			tokens:   make(map[string]map[string]int),
			noteKeys: make(map[string]map[string]int),
			pins:     make(map[string]time.Time),
		}
		for _, note := range notes {
			if err := idx.Upsert(note); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkSearch_10k(b *testing.B) {
	svc := benchmarkServiceWithNotes(b, 10_000)
	b.ResetTimer()
	for b.Loop() {
		results, err := svc.Search("benchmark 09999", 50)
		if err != nil || len(results) == 0 {
			b.Fatalf("Search: %v (%d results)", err, len(results))
		}
	}
}

func BenchmarkBacklinks_10k(b *testing.B) {
	svc := benchmarkServiceWithNotes(b, 10_000)
	b.ResetTimer()
	for b.Loop() {
		if _, err := svc.GetBacklinks("benchmark note 09999", "", 50); err != nil {
			b.Fatalf("GetBacklinks: %v", err)
		}
	}
}

func benchmarkServiceWithNotes(b *testing.B, count int) *Service {
	b.Helper()
	dir := b.TempDir()
	svc, err := New(Options{Root: dir, StartWatcher: false})
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	b.Cleanup(func() { _ = svc.Close() })

	idx, ok := svc.index.(*memoryIndex)
	if !ok {
		b.Fatalf("unexpected index type %T", svc.index)
	}
	for _, note := range benchmarkNotes(count) {
		if err := idx.Upsert(note); err != nil {
			b.Fatalf("seed benchmark: %v", err)
		}
	}
	return svc
}

func benchmarkNotes(count int) []domain.Note {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	notes := make([]domain.Note, 0, count)
	for i := 0; i < count; i++ {
		folder := fmt.Sprintf("notes/folder-%03d/section-%02d", i%100, i%10)
		content := fmt.Sprintf("# Benchmark Note %05d\n\nContent for benchmark note %05d.", i, i)
		notes = append(notes, domain.Note{
			RelativePath: fmt.Sprintf("%s/note-%05d.md", folder, i),
			Title:        fmt.Sprintf("Benchmark Note %05d", i),
			Content:      content,
			CreatedAt:    now,
			UpdatedAt:    now.Add(time.Duration(i) * time.Second),
		})
	}
	return notes
}
