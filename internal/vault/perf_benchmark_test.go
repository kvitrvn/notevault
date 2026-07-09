package vault

import (
	"fmt"
	"testing"
	"time"
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

func benchmarkServiceWithNotes(b *testing.B, count int) *Service {
	b.Helper()
	dir := b.TempDir()
	svc, err := New(Options{Root: dir, StartWatcher: false})
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	b.Cleanup(func() { _ = svc.Close() })

	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	idx, ok := svc.index.(*sqliteIndex)
	if !ok {
		b.Fatalf("unexpected index type %T", svc.index)
	}
	tx, err := idx.db.Begin()
	if err != nil {
		b.Fatalf("begin benchmark seed: %v", err)
	}
	stmt, err := tx.Prepare(`
        INSERT INTO notes (relative_path, title, content, size, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?)
    `)
	if err != nil {
		_ = tx.Rollback()
		b.Fatalf("prepare benchmark seed: %v", err)
	}
	defer stmt.Close()
	for i := 0; i < count; i++ {
		folder := fmt.Sprintf("notes/folder-%03d/section-%02d", i%100, i%10)
		content := fmt.Sprintf("# Benchmark Note %05d\n\nContent for benchmark note %05d.", i, i)
		_, err := stmt.Exec(
			fmt.Sprintf("%s/note-%05d.md", folder, i),
			fmt.Sprintf("Benchmark Note %05d", i),
			content,
			len(content),
			now.Unix(),
			now.Add(time.Duration(i)*time.Second).Unix(),
		)
		if err != nil {
			_ = tx.Rollback()
			b.Fatalf("insert benchmark note: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatalf("commit benchmark seed: %v", err)
	}
	return svc
}
