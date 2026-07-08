package vault

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/votre-compte/notevault/internal/domain"
)

// progressReporter reçoit les étapes d'indexation initiale.
type progressReporter interface {
	OnProgress(stage string, current, total int)
}

// Watcher observe les changements dans le coffre et synchronise l'index.
type Watcher struct {
	root    string
	index   Index
	fs      *fsnotify.Watcher
	stop    chan struct{}
	done    chan struct{}
	reindex func(path string)
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
	defer close(w.done)
	timers := make(map[string]*time.Timer)
	var timersMu sync.Mutex
	schedule := func(path string) {
		if isIgnored(path) {
			return
		}
		timersMu.Lock()
		if t, ok := timers[path]; ok {
			t.Stop()
		}
		timers[path] = time.AfterFunc(watcherDebounce, func() {
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

// IndexExisting scanne le dossier notes/ et alimente l'index.
// Émet la progression via reporter.
func IndexExisting(ctx context.Context, root string, idx Index, reporter progressReporter) error {
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
		return fmt.Errorf("lister les notes existantes : %w", err)
	}
	total := len(files)
	if reporter != nil {
		reporter.OnProgress("index", 0, total)
	}
	for i, path := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		note, err := readAbsoluteFile(root, path)
		if err != nil {
			continue
		}
		if err := idx.Upsert(note); err != nil {
			return fmt.Errorf("indexer %s : %w", path, err)
		}
		if reporter != nil && (i%100 == 0 || i == total-1) {
			reporter.OnProgress("index", i+1, total)
		}
	}
	if reporter != nil {
		reporter.OnProgress("index", total, total)
	}
	return nil
}

func readAbsoluteFile(root, absPath string) (domain.Note, error) {
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return domain.Note{}, fmt.Errorf("lire la note : %w", err)
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return domain.Note{}, err
	}
	note := parse(string(raw))
	note.RelativePath = filepath.ToSlash(rel)
	if note.Title == "" {
		note.Title = strings.TrimSuffix(filepath.Base(absPath), filepath.Ext(absPath))
	}
	info, err := os.Stat(absPath)
	if err == nil && note.UpdatedAt.IsZero() {
		note.UpdatedAt = info.ModTime().UTC()
	}
	return note, nil
}
