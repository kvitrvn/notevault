package vault

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kvitrvn/notevault/internal/domain"
)

// TestIndexFilesParallelReadsConcurrently vérifie que indexFiles exécute
// effectivement les lectures en parallèle : le reader est artificiellement
// lent, et on observe plusieurs goroutines actives en même temps.
func TestIndexFilesParallelReadsConcurrently(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	idx, err := newMemoryIndex(dir)
	if err != nil {
		t.Fatalf("newMemoryIndex: %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	const files = 32
	const perRead = 20 * time.Millisecond
	const workers = 8

	var concurrent atomic.Int32
	var peak atomic.Int32

	paths := make([]string, files)
	for i := range paths {
		paths[i] = fmt.Sprintf("/tmp/fake-note-%02d.md", i)
	}

	reader := func(path string) (domain.Note, error) {
		now := concurrent.Add(1)
		for {
			old := peak.Load()
			if now <= old || peak.CompareAndSwap(old, now) {
				break
			}
		}
		time.Sleep(perRead)
		concurrent.Add(-1)
		return domain.Note{
			RelativePath: path,
			Title:        path,
			Content:      "body",
			CreatedAt:    time.Unix(0, 0).UTC(),
			UpdatedAt:    time.Unix(0, 0).UTC(),
		}, nil
	}

	start := time.Now()
	if err := indexFiles(context.Background(), dir, idx, paths, nil, reader); err != nil {
		t.Fatalf("indexFiles: %v", err)
	}
	elapsed := time.Since(start)

	if peak.Load() < 2 {
		t.Fatalf("aucun parallélisme détecté, pic=%d (sequential)", peak.Load())
	}
	if peak.Load() > int32(workers)+1 {
		t.Fatalf("pool plus large que prévu, pic=%d workers=%d", peak.Load(), workers)
	}
	// Sanity : on doit observer un gain par rapport au strict séquentiel.
	sequential := time.Duration(files) * perRead
	if elapsed >= sequential {
		t.Fatalf("durée %s ≥ séquentiel %s, parallélisme sans effet", elapsed, sequential)
	}
}

func TestIndexFilesHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	idx, err := newMemoryIndex(dir)
	if err != nil {
		t.Fatalf("newMemoryIndex: %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	const files = 64
	paths := make([]string, files)
	for i := range paths {
		paths[i] = fmt.Sprintf("/tmp/cancel-%03d.md", i)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	reader := func(path string) (domain.Note, error) {
		// Lecteur lent : garantit que ctx peut être annulé pendant le vol.
		select {
		case <-ctx.Done():
			return domain.Note{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
			return domain.Note{RelativePath: path, Title: path}, nil
		}
	}

	go func() {
		time.Sleep(25 * time.Millisecond)
		cancel()
	}()

	err = indexFiles(ctx, dir, idx, paths, nil, reader)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("attendu context.Canceled, observé %v", err)
	}
}

func TestIndexFilesEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	idx, err := newMemoryIndex(dir)
	if err != nil {
		t.Fatalf("newMemoryIndex: %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	calls := 0
	reader := func(path string) (domain.Note, error) {
		calls++
		return domain.Note{}, nil
	}
	if err := indexFiles(context.Background(), dir, idx, nil, nil, reader); err != nil {
		t.Fatalf("indexFiles: %v", err)
	}
	if calls != 0 {
		t.Fatalf("reader appelé %d fois sur fichier vide", calls)
	}
}

func TestIndexWorkerCountScalesWithTotal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		total   int
		wantMin int
		wantMax int
	}{
		{0, 1, 1},
		{1, 1, 1},
		{16, 2, 8},
		{10_000, 2, 8},
	}
	for _, tc := range cases {
		got := indexWorkerCount(tc.total)
		if got < tc.wantMin || got > tc.wantMax {
			t.Errorf("indexWorkerCount(%d) = %d, want dans [%d,%d]", tc.total, got, tc.wantMin, tc.wantMax)
		}
	}
}
