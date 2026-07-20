package vault

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kvitrvn/notevault/internal/domain"
)

// progressReporter reçoit les étapes d'indexation initiale.
type progressReporter interface {
	OnProgress(stage string, current, total int)
}

type reconcileIndex interface {
	Index
	ListPaths() ([]string, error)
	SetMeta(key, value string) error
}

const indexMetaLastFullIndexAt = "last_full_index_at"

// Watcher observe les changements dans le coffre et synchronise l'index.
type Watcher struct {
	root      string
	index     Index
	fs        *fsnotify.Watcher
	stop      chan struct{}
	done      chan struct{}
	reindex   func(path string)
	callbacks sync.WaitGroup
}

const watcherDebounce = 200 * time.Millisecond

// NewWatcher crée et démarre un watcher. Reindex est appelé pour chaque
// fichier modifié/créé ; il doit re-parser le fichier et appeler
// Index.Upsert/Delete selon le cas.
func NewWatcher(ctx context.Context, root string, idx Index, reindex func(path string)) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("créer le watcher fsnotify : %w", err)
	}
	notesRoot := filepath.Join(root, "notes")
	if err := fsw.Add(notesRoot); err != nil {
		fsw.Close()
		return nil, fmt.Errorf("surveiller %s : %w", notesRoot, err)
	}
	if err := watchRecursive(fsw, notesRoot); err != nil {
		fsw.Close()
		return nil, err
	}

	w := &Watcher{
		root:    root,
		index:   idx,
		fs:      fsw,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		reindex: reindex,
	}
	go w.loop(ctx)
	return w, nil
}

func watchRecursive(fsw *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			if err := fsw.Add(path); err != nil {
				return err
			}
		}
		return nil
	})
}

// Close arrête le watcher.
func (w *Watcher) Close() error {
	close(w.stop)
	<-w.done
	return w.fs.Close()
}

func (w *Watcher) loop(ctx context.Context) {
	timers := make(map[string]*time.Timer)
	var timersMu sync.Mutex
	defer func() {
		timersMu.Lock()
		for _, timer := range timers {
			if timer.Stop() {
				w.callbacks.Done()
			}
		}
		timersMu.Unlock()
		w.callbacks.Wait()
		close(w.done)
	}()
	schedule := func(path string) {
		if isIgnored(path) {
			return
		}
		timersMu.Lock()
		if timer, ok := timers[path]; ok && timer.Stop() {
			w.callbacks.Done()
		}
		w.callbacks.Add(1)
		timers[path] = time.AfterFunc(watcherDebounce, func() {
			defer w.callbacks.Done()
			w.reindex(path)
		})
		timersMu.Unlock()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stop:
			return
		case ev, ok := <-w.fs.Events:
			if !ok {
				return
			}
			if isIgnored(ev.Name) {
				continue
			}
			if ev.Op&fsnotify.Create == fsnotify.Create {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					_ = w.fs.Add(ev.Name)
				}
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				schedule(ev.Name)
			}
		case _, ok := <-w.fs.Errors:
			if !ok {
				return
			}
		}
	}
}

func isIgnored(path string) bool {
	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") {
		return true
	}
	if strings.HasSuffix(base, "~") ||
		strings.HasSuffix(base, ".tmp") ||
		strings.HasSuffix(base, ".swp") ||
		strings.HasSuffix(base, ".meta") ||
		base == "4913" {
		return true
	}
	return false
}

func indexExistingWithReader(ctx context.Context, root string, idx Index, reporter progressReporter, reader func(string) (domain.Note, error)) error {
	files, err := markdownFiles(root)
	if err != nil {
		return err
	}
	return indexFiles(ctx, root, idx, files, reporter, reader)
}

func reconcileExistingWithReader(ctx context.Context, root string, idx reconcileIndex, reporter progressReporter, reader func(string) (domain.Note, error)) error {
	files, err := markdownFiles(root)
	if err != nil {
		return err
	}

	diskPaths := make(map[string]struct{}, len(files))
	for _, path := range files {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		diskPaths[filepath.ToSlash(rel)] = struct{}{}
	}

	indexed, err := idx.ListPaths()
	if err != nil {
		return err
	}
	for _, rel := range indexed {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if _, ok := diskPaths[rel]; ok {
			continue
		}
		if err := idx.Delete(rel); err != nil && !errors.Is(err, ErrNotFound) {
			return fmt.Errorf("supprimer l'entrée d'index obsolète %s : %w", rel, err)
		}
	}

	if err := indexFiles(ctx, root, idx, files, reporter, reader); err != nil {
		return err
	}
	return idx.SetMeta(indexMetaLastFullIndexAt, nowUTC().Format(time.RFC3339))
}

func markdownFiles(root string) ([]string, error) {
	notesRoot := filepath.Join(root, "notes")
	files := make([]string, 0)
	err := filepath.WalkDir(notesRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("lister les notes existantes : %w", err)
	}
	return files, nil
}

func indexFiles(ctx context.Context, root string, idx Index, files []string, reporter progressReporter, reader func(string) (domain.Note, error)) error {
	total := len(files)
	if reporter != nil {
		reporter.OnProgress("index", 0, total)
	}
	if total == 0 {
		return nil
	}

	// Les lectures (I/O disque + decrypt éventuel) sont parallélisées via
	// un pool borné : sur les gros coffres, cela évite que IndexNow soit
	// dominé par des lectures séquentielles. Les Upsert/Delete restent
	// sérialisés par le mutex interne de l'index pour ne pas dégrader les
	// requêtes concurrentes (audit perf 2.3).
	workers := indexWorkerCount(total)
	type readResult struct {
		path string
		note domain.Note
		err  error
	}
	jobs := make(chan string, workers)
	results := make(chan readResult, workers)

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for path := range jobs {
				if ctx.Err() != nil {
					return
				}
				note, err := reader(path)
				select {
				case <-ctx.Done():
					return
				case results <- readResult{path: path, note: note, err: err}:
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, path := range files {
			select {
			case <-ctx.Done():
				return
			case jobs <- path:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	processed := 0
	for r := range results {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if r.err != nil {
			rel, _ := filepath.Rel(root, r.path)
			rel = filepath.ToSlash(rel)
			if deleteErr := idx.Delete(rel); deleteErr != nil && !errors.Is(deleteErr, ErrNotFound) {
				return deleteErr
			}
		} else {
			if err := idx.Upsert(r.note); err != nil {
				return fmt.Errorf("indexer %s : %w", r.path, err)
			}
		}
		processed++
		if reporter != nil && (processed%100 == 0 || processed == total) {
			reporter.OnProgress("index", processed, total)
		}
	}
	if reporter != nil {
		reporter.OnProgress("index", total, total)
	}
	return nil
}

// indexWorkerCount dimensionne le pool de lecteurs. On borne à 8 pour
// éviter de saturer le disque sur les machines multi-cœurs ; le minimum
// à 2 évite la régression sur les contextes mono-thread (tests, sandbox).
func indexWorkerCount(total int) int {
	if total < 2 {
		return 1
	}
	n := runtime.NumCPU()
	if n < 2 {
		n = 2
	}
	if n > 8 {
		n = 8
	}
	return n
}
