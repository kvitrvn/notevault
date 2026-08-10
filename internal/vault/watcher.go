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

// fsEventKind est l'intention déduite d'un événement fsnotify. Le
// service peut encore ignorer ou compléter l'action en fonction de
// l'état du disque.
type fsEventKind int

const (
	fsEventUpsert fsEventKind = iota
	fsEventRemove
)

type fsObservation struct {
	path  string
	kind  fsEventKind
	isDir bool
}

// pathProcessor consomme un batch d'observations du watcher et produit
// la liste des changements réellement appliqués à l'index. fullRescan
// est vrai quand l'index doit être considéré comme incohérent (perte
// d'événements fsnotify, débordement, état initial).
type pathProcessor interface {
	ApplyFsEvents(observations []fsObservation) (changes []VaultChange, fullRescan bool, err error)
}

const watcherDebounce = 200 * time.Millisecond

// Watcher observe les changements dans le coffre et synchronise l'index.
// Il agit comme un coalesceur : plusieurs événements rapprochés sur le
// même chemin ne déclenchent qu'une seule action par fenêtre de
// debounce. Les créations de dossiers étendent récursivement le
// périmètre surveillé ; les suppressions utilisent DeletePrefix.
type Watcher struct {
	root      string
	fs        *fsnotify.Watcher
	stop      chan struct{}
	done      chan struct{}
	processor pathProcessor
}

type watcherOptions struct {
	notesRoot string
}

// NewWatcher crée et démarre le watcher sur le sous-dossier notes/.
// processor.ApplyFsEvents est appelé pour chaque batch coalescé ; il
// est responsable de la lecture disque et de l'index. Si le watcher ne
// peut pas s'initialiser, le caller doit décider de continuer sans lui.
func NewWatcher(ctx context.Context, root string, processor pathProcessor) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("créer le watcher fsnotify : %w", err)
	}
	notesRoot := filepath.Join(root, "notes")
	if _, err := os.Stat(notesRoot); err != nil {
		fsw.Close()
		return nil, fmt.Errorf("accéder à %s : %w", notesRoot, err)
	}
	if err := fsw.Add(notesRoot); err != nil {
		fsw.Close()
		return nil, fmt.Errorf("surveiller %s : %w", notesRoot, err)
	}
	if err := watchRecursive(fsw, notesRoot); err != nil {
		fsw.Close()
		return nil, err
	}

	w := &Watcher{
		root:      root,
		fs:        fsw,
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
		processor: processor,
	}
	go w.loop(ctx)
	return w, nil
}

// Close arrête le watcher et libère fsnotify.
func (w *Watcher) Close() error {
	select {
	case <-w.stop:
	default:
		close(w.stop)
	}
	<-w.done
	return w.fs.Close()
}

// watchRecursive ajoute le dossier racine et tous ses sous-dossiers au
// watcher fsnotify. Appelé au démarrage et lors de chaque création de
// dossier détectée à chaud.
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

// loop accumule les événements dans pending (dédupliqués par chemin)
// et déclenche un batch à intervalles réguliers. Chaque dossier
// créé déclenche un scan récursif pour ne pas attendre les inodes
// enfants ; chaque dossier retiré déclenche DeletePrefix.
func (w *Watcher) loop(ctx context.Context) {
	defer close(w.done)
	pending := make(map[string]fsObservation)
	var pendingMu sync.Mutex
	var timer *time.Timer
	var timerC <-chan time.Time

	schedule := func() {
		if timer != nil {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		timer = time.NewTimer(watcherDebounce)
		timerC = timer.C
	}

	flush := func(reason string) {
		pendingMu.Lock()
		if len(pending) == 0 {
			pendingMu.Unlock()
			return
		}
		batch := make([]fsObservation, 0, len(pending))
		for _, obs := range pending {
			batch = append(batch, obs)
		}
		pending = make(map[string]fsObservation)
		pendingMu.Unlock()

		if timer != nil {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		timer = nil
		timerC = nil

		_ = reason
		if _, _, err := w.processor.ApplyFsEvents(batch); err != nil {
			// L'échec du processor n'est pas un crash du watcher : il sera
			// retenté à la prochaine fenêtre grâce au rescan périodique.
			// On logge uniquement si on a un logger ; ici on garde le
			// silence car l'application n'expose pas encore de logger
			// central, et IndexNow() est le filet de récupération.
			_ = err
		}
	}

	for {
		select {
		case <-ctx.Done():
			flush("ctx")
			return
		case <-w.stop:
			flush("stop")
			return
		case ev, ok := <-w.fs.Events:
			if !ok {
				flush("events-closed")
				return
			}
			if isIgnored(ev.Name) {
				continue
			}
			obs, ok := classifyEvent(ev)
			if !ok {
				continue
			}
			// Dossier : on étend récursivement la surveillance AVANT de
			// scheduler le batch, pour que les fichiers enfants soient
			// déjà connus de fsnotify quand leur événement arrivera.
			if obs.isDir && obs.kind == fsEventUpsert {
				if info, err := os.Stat(obs.path); err == nil && info.IsDir() {
					if err := watchRecursive(w.fs, obs.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
						_ = err
					}
				} else {
					// Le dossier n'existe pas : probablement déjà retiré.
					obs.kind = fsEventRemove
				}
			}
			// Dossier retiré : on n'a plus besoin de le surveiller.
			if obs.isDir && obs.kind == fsEventRemove {
				_ = w.fs.Remove(obs.path)
			}

			pendingMu.Lock()
			previous, exists := pending[obs.path]
			// Remove écrase Upsert (priorité au retrait).
			if !exists || previous.kind == fsEventUpsert || obs.kind == fsEventRemove {
				pending[obs.path] = obs
			}
			pendingMu.Unlock()
			schedule()
		case err, ok := <-w.fs.Errors:
			if !ok {
				flush("errors-closed")
				return
			}
			if err == nil {
				continue
			}
			// Erreur fsnotify (souvent overflow) : on force un batch avec
			// une observation sentinel qui demandera un rescan complet.
			pendingMu.Lock()
			pending["__rescan__"] = fsObservation{path: fsRescanSentinel, kind: fsEventUpsert}
			pendingMu.Unlock()
			schedule()
		case <-timerC:
			flush("debounce")
			timer = nil
			timerC = nil
		}
	}
}

// classifyEvent traduit un événement brut en observation pour notre
// modèle upsert/remove. Renvoie ok=false pour les opérations
// inintéressantes (Chmod sans contenu modifié).
func classifyEvent(ev fsnotify.Event) (fsObservation, bool) {
	if ev.Name == "" {
		return fsObservation{}, false
	}
	info, err := os.Lstat(ev.Name)
	isDir := err == nil && info.IsDir()
	// Remove : on n'a pas d'info disque fiable, on tente Lstat ; s'il
	// échoue on suppose "fichier" plutôt que de risquer une mauvaise
	// classification silencieuse.
	switch {
	case ev.Op&(fsnotify.Remove|fsnotify.Rename) != 0:
		return fsObservation{path: ev.Name, kind: fsEventRemove, isDir: isDir}, true
	case ev.Op&(fsnotify.Create|fsnotify.Write) != 0:
		return fsObservation{path: ev.Name, kind: fsEventUpsert, isDir: isDir}, true
	default:
		return fsObservation{}, false
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

// indexExistingWithReader et reconcileExistingWithReader restent les
// points d'entrée du bootstrap initial. Ils sont inchangés pour cette
// PR : la fusion avec le bus de changements viendra en 0.4.4.

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
