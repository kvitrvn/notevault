package vault

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kvitrvn/notevault/internal/config"
	"github.com/kvitrvn/notevault/internal/domain"
)

// Service est la façade métier du coffre. Elle combine un index mémoire
// (cache requêtable), un Store de configuration et des opérations fichiers
// atomiques. Toute mutation doit passer par elle pour rester cohérente.
type Service struct {
	root      string
	index     Index
	config    *config.Store
	state     *stateStore
	watcher   *Watcher
	templates *TemplateLoader
	themes    *ThemeLoader
	indexCtx  context.Context
	indexStop context.CancelFunc

	securityMu         sync.RWMutex
	mutationMu         sync.RWMutex
	vaultState         VaultState
	metadata           *encryptionMetadata
	metadataInvalid    bool
	vaultKey           []byte
	encryptionWarnings []string
	migrationCurrent   int
	migrationTotal     int
	watcherWanted      bool

	recentWriteMu sync.Mutex
	// recentWrites mémorise les chemins absolus que le service vient
	// d'écrire, pour ignorer l'écho du watcher fsnotify (cf. audit 2.1).
	// Consommé/consommé-expire automatiquement par consumeInternalWrite.
	recentWrites map[string]time.Time
}

// recentWriteWindow borne la durée pendant laquelle une écriture interne
// est considérée comme « récente » et déclenche un skip de reindex.
// Doit rester largement supérieur au debounce du watcher (200 ms).
const recentWriteWindow = 5 * time.Second

// recentWriteCleanupThreshold borne la taille de recentWrites avant un
// nettoyage opportuniste des entrées expirées.
const recentWriteCleanupThreshold = 32

// Options configure la construction du service.
type Options struct {
	Root         string
	Index        Index         // si nil, un index mémoire est créé
	Config       *config.Store // si nil, un Store est créé
	StartWatcher bool
}

// New construit un service à partir d'options explicites (utilisé par les tests).
func New(opts Options) (*Service, error) {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, fmt.Errorf("résoudre la racine du coffre : %w", err)
	}
	if err := ensureDirs(root); err != nil {
		return nil, err
	}
	if err := ensureTemplateDir(root); err != nil {
		return nil, err
	}
	if err := ensureThemeDir(root); err != nil {
		return nil, err
	}
	idx := opts.Index
	if idx == nil {
		idx, err = newMemoryIndex(root)
		if err != nil {
			return nil, err
		}
		for _, suffix := range []string{"", "-wal", "-shm"} {
			_ = os.Remove(filepath.Join(root, ".notevault", "index.db") + suffix)
		}
	}
	cfg := opts.Config
	if cfg == nil {
		cfg = config.NewStore(root)
	}
	s := &Service{
		root:          root,
		index:         idx,
		config:        cfg,
		state:         newStateStore(root),
		templates:     NewTemplateLoader(root),
		themes:        NewThemeLoader(root),
		vaultState:    VaultDisabled,
		watcherWanted: opts.StartWatcher,
	}
	metadata, err := loadEncryptionMetadata(root)
	if err != nil {
		if errors.Is(err, ErrUnlockFailed) {
			s.metadataInvalid = true
			s.vaultState = VaultLocked
		} else {
			return nil, err
		}
	}
	if metadata != nil {
		s.metadata = metadata
		s.vaultState = VaultLocked
		s.encryptionWarnings = append([]string(nil), metadata.Warnings...)
	}
	// Purge de la corbeille au démarrage.
	if loaded, err := cfg.Load(); err == nil {
		_ = purgeTrash(root, loaded.TrashRetentionDays)
	} else {
		_ = purgeTrash(root, config.Default().TrashRetentionDays)
	}
	if opts.StartWatcher && s.vaultState == VaultDisabled {
		if err := s.startWatcher(); err != nil {
			idx.Close()
			return nil, err
		}
	}
	return s, nil
}

// NewDefaultService crée le service avec les options par défaut.
// L'indexation initiale n'est PAS lancée ici : elle reste à la charge de
// l'appelant via IndexNow (synchrone, parallélisé en lecture). Le watcher
// est démarré pour capter les modifications ultérieures.
// Si AutoDailyNote est activé, crée la note du jour si absente et
// retourne son chemin via BootstrapDailyNote().
func NewDefaultService() (*Service, error) {
	root, err := defaultVaultPath()
	if err != nil {
		return nil, err
	}
	svc, err := New(Options{Root: root, StartWatcher: true})
	if err != nil {
		return nil, fmt.Errorf("initialiser le coffre : %w", err)
	}
	return svc, nil
}

// EnsureDailyNote crée la note du jour (notes/daily/YYYY-MM-DD.md) si
// la config AutoDailyNote est vraie et qu'elle n'existe pas encore.
// Retourne le chemin relatif de la note du jour.
func (s *Service) EnsureDailyNote() (string, error) {
	cfg, err := s.config.Load()
	if err != nil {
		return "", err
	}
	if !cfg.AutoDailyNote {
		return "", nil
	}
	return s.ensureDailyNoteImpl()
}

func (s *Service) ensureDailyNoteImpl() (string, error) {
	now := nowUTC()
	day := now.Format("2006-01-02")
	rel := filepath.ToSlash(filepath.Join("notes", "daily", day+".md"))
	path, err := s.absoluteNotePath(rel)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return rel, nil
	}
	note := domain.Note{
		RelativePath: rel,
		Title:        "Journal — " + day,
		Content:      template("daily"),
		CreatedAt:    now,
		UpdatedAt:    now,
		Tags:         []string{"daily"},
	}
	if _, err := s.SaveNote(note); err != nil {
		return "", err
	}
	return rel, nil
}

// Root retourne la racine absolue du coffre.
func (s *Service) Root() string { return s.root }

// BootstrapContext retourne un contexte lié à la durée de vie du service.
// Utilisé pour les opérations longues (indexation initiale) qui doivent
// pouvoir être annulées par Close().
func (s *Service) BootstrapContext() context.Context {
	if s.indexCtx != nil {
		return s.indexCtx
	}
	return context.Background()
}

// Close ferme les ressources ouvertes.
func (s *Service) Close() error {
	s.watcherWanted = false
	s.stopWatcher()
	s.securityMu.Lock()
	zeroBytes(s.vaultKey)
	s.vaultKey = nil
	s.securityMu.Unlock()
	if s.index != nil {
		return s.index.Close()
	}
	return nil
}

// IndexNow réconcilie l'index avec le contenu réel du dossier notes/.
// Synchrone : utilisé au bootstrap et en cas de récupération après crash.
// L'opération ajoute les fichiers absents de l'index, met à jour les entrées
// existantes, supprime les chemins dont le fichier a disparu et enregistre
// meta.last_full_index_at quand le backend d'index le permet.
func (s *Service) IndexNow(ctx context.Context, reporter progressReporter) error {
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	if idx, ok := s.index.(reconcileIndex); ok {
		return reconcileExistingWithReader(ctx, s.root, idx, reporter, s.readForIndex)
	}
	return indexExistingWithReader(ctx, s.root, s.index, reporter, s.readForIndex)
}

func (s *Service) readForIndex(path string) (domain.Note, error) {
	note, err := s.readAbsolute(path)
	if err != nil {
		if rel, relErr := filepath.Rel(s.root, path); relErr == nil {
			s.addEncryptionWarning(filepath.ToSlash(rel) + " : note illisible")
		}
	}
	return note, err
}

func (s *Service) reindexFromPath(absPath string) {
	// Détermine le chemin relatif à la racine du coffre.
	rel, err := filepath.Rel(s.root, absPath)
	if err != nil {
		return
	}
	rel = filepath.ToSlash(rel)
	if !strings.HasPrefix(rel, "notes/") {
		// Hors de la zone indexée : on supprime si c'était une note connue.
		_ = s.index.Delete(rel)
		return
	}
	if strings.ToLower(filepath.Ext(absPath)) != ".md" {
		return
	}
	if _, err := os.Stat(absPath); err != nil {
		// Fichier supprimé.
		_ = s.index.Delete(rel)
		s.consumeInternalWrite(absPath)
		return
	}
	// Ignorer l'écho d'une écriture interne récente : SaveNote,
	// MoveNote et RestoreFromTrash ont déjà réindexé en mémoire juste
	// après l'écriture atomique. Sans ce court-circuit, chaque sauvegarde
	// déclencherait une seconde lecture complète + re-tokenisation par le
	// watcher (cf. audit perf 2.1).
	if _, ok := s.consumeInternalWrite(absPath); ok {
		return
	}
	note, err := s.readAbsolute(absPath)
	if err != nil {
		_ = s.index.Delete(rel)
		s.addEncryptionWarning(rel + " : note illisible")
		return
	}
	_ = s.index.Upsert(note)
}

// markInternalWrite enregistre absPath comme venant d'être écrit par
// le service. Le watcher fsnotify observe ses propres écritures : cette
// marque permet à reindexFromPath d'ignorer l'écho pendant recentWriteWindow.
func (s *Service) markInternalWrite(absPath string) {
	s.recentWriteMu.Lock()
	defer s.recentWriteMu.Unlock()
	if s.recentWrites == nil {
		s.recentWrites = make(map[string]time.Time)
	}
	s.recentWrites[absPath] = time.Now()
	if len(s.recentWrites) > recentWriteCleanupThreshold {
		cutoff := time.Now().Add(-recentWriteWindow)
		for path, t := range s.recentWrites {
			if t.Before(cutoff) {
				delete(s.recentWrites, path)
			}
		}
	}
}

// consumeInternalWrite consulte et retire la marque d'écriture pour
// absPath. Retourne (t, true) si une écriture récente par le service
// est confirmée : reindexFromPath doit alors ignorer la note (l'index est
// déjà à jour). Retourne (zero, false) sinon, ou si la marque a expiré.
func (s *Service) consumeInternalWrite(absPath string) (time.Time, bool) {
	s.recentWriteMu.Lock()
	defer s.recentWriteMu.Unlock()
	t, ok := s.recentWrites[absPath]
	if !ok {
		return time.Time{}, false
	}
	if time.Since(t) > recentWriteWindow {
		delete(s.recentWrites, absPath)
		return time.Time{}, false
	}
	delete(s.recentWrites, absPath)
	return t, true
}

func (s *Service) ListNotes() ([]domain.NoteSummary, error) {
	return s.ListNotesFiltered(FilterQuery{}, 5000)
}

// ListNotesFiltered applique une requête structurée et retourne les
// résumés de notes correspondants. Si la requête est vide, équivaut
// à ListNotes().
func (s *Service) ListNotesFiltered(q FilterQuery, limit int) ([]domain.NoteSummary, error) {
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 5000
	}
	summaries, err := s.index.List(q.ToListFilter(limit))
	if err != nil {
		return nil, fmt.Errorf("lister les notes : %w", err)
	}
	if q.IsEmpty() {
		// Tri stable déjà assuré par l'index.
		return summaries, nil
	}
	// Tri déjà assuré par l'index.
	return summaries, nil
}

// ListPinned retourne les notes épinglées (par ordre d'épinglage).
func (s *Service) ListPinned() ([]domain.NoteSummary, error) {
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	out, err := s.index.ListPinned()
	if err != nil {
		return nil, fmt.Errorf("lister les épinglées : %w", err)
	}
	return out, nil
}

// ListFolders retourne les dossiers visibles dans la vue arborescente :
// union des dossiers dérivés des notes indexées et des dossiers vides
// présents physiquement sous notes/ (créés via l'app ou un explorateur).
// Les dossiers sont dédupliqués et triés par chemin.
func (s *Service) ListFolders() ([]FolderInfo, error) {
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	indexed, err := s.index.ListFolders()
	if err != nil {
		return nil, fmt.Errorf("lister les dossiers indexés : %w", err)
	}
	byPath := make(map[string]int, len(indexed))
	for _, f := range indexed {
		byPath[f.Path] = f.Count
	}
	onDisk, err := scanNotesFolders(s.root, 8)
	if err != nil {
		return nil, fmt.Errorf("lister les dossiers présents : %w", err)
	}
	for _, p := range onDisk {
		if _, ok := byPath[p]; !ok {
			byPath[p] = 0
		}
	}
	out := make([]FolderInfo, 0, len(byPath))
	for path, count := range byPath {
		out = append(out, FolderInfo{Path: path, Name: filepath.Base(path), Count: count})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Path < out[b].Path })
	return out, nil
}

// Pin épingle ou désépingle une note.
func (s *Service) Pin(relativePath string, pinned bool) error {
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	if _, err := s.absoluteNotePath(relativePath); err != nil {
		return err
	}
	return s.index.Pin(relativePath, pinned)
}

// IsPinned retourne l'état d'épinglage d'une note.
func (s *Service) IsPinned(relativePath string) (bool, error) {
	if err := s.requireUnlocked(); err != nil {
		return false, err
	}
	return s.index.IsPinned(relativePath)
}

// ListTags retourne tous les tags avec leur compte.
func (s *Service) ListTags() ([]TagCount, error) {
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	return s.index.ListTags()
}

// Search interroge l'index full-text.
func (s *Service) Search(query string, limit int) ([]domain.NoteSummary, error) {
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	return s.index.Search(query, SearchOpts{Limit: limit})
}

func (s *Service) OpenNote(relativePath string) (domain.Note, error) {
	path, err := s.absoluteNotePath(relativePath)
	if err != nil {
		return domain.Note{}, err
	}
	return s.readAbsolute(path)
}

// CreateNote crée une nouvelle note dans le dossier indiqué. parentRelPath
// est un chemin relatif (par exemple "inbox", "projets" ou "projets/web"),
// ou une chaîne vide pour utiliser le dossier par défaut (notes/inbox/).
// Le nom de fichier reste au format YYYYMMDD-HHMMSS-slug.md pour rester
// cohérent avec le reste de l'app.
func (s *Service) CreateNote(parentRelPath, title, templateKey string) (domain.Note, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Nouvelle note"
	}
	parent, err := s.resolveNoteParent(parentRelPath, "notes/inbox")
	if err != nil {
		return domain.Note{}, err
	}

	now := nowUTC()
	filename := fmt.Sprintf("%s-%s.md", now.Format("20060102-150405"), slug(title))
	relativePath := filepath.ToSlash(filepath.Join(parent, filename))
	content := s.resolveTemplateBody(templateKey)

	note := domain.Note{
		RelativePath: relativePath,
		Title:        title,
		Content:      content,
		CreatedAt:    now,
		UpdatedAt:    now,
		Tags:         []string{},
	}
	return s.SaveNote(note)
}

// CreateFolder crée un nouveau dossier vide sous notes/. parentRelPath est
// un chemin relatif (par exemple "projets" ou "projets/web"), ou une chaîne
// vide pour la racine notes/. name est slugifié pour garantir un nom de
// fichier/dossier sûr et stable. Retourne ErrFolderExists si le dossier
// est déjà présent.
func (s *Service) CreateFolder(parentRelPath, name string) (domain.Note, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return domain.Note{}, err
	}
	parent, err := s.resolveNoteParent(parentRelPath, "notes")
	if err != nil {
		return domain.Note{}, err
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || strings.ContainsAny(trimmed, "/\\") || strings.Contains(trimmed, "..") {
		return domain.Note{}, errors.New("nom de dossier invalide")
	}
	folderSlug := slug(trimmed)
	// slug renvoie "note" en fallback pour les chaînes vides après
	// nettoyage. On le traite comme invalide quand le nom d'origine est
	// non vide mais ne contient aucun caractère exploitable (ex: "!!!").
	if folderSlug == "" || (folderSlug == "note" && !strings.EqualFold(trimmed, "note")) {
		return domain.Note{}, errors.New("nom de dossier invalide")
	}
	relFolder := filepath.ToSlash(filepath.Join(parent, folderSlug))
	absFolder := filepath.Join(s.root, relFolder)
	if info, err := os.Stat(absFolder); err == nil {
		if info.IsDir() {
			return domain.Note{}, ErrFolderExists
		}
		return domain.Note{}, errors.New("un fichier porte déjà ce nom")
	} else if !errors.Is(err, os.ErrNotExist) {
		return domain.Note{}, fmt.Errorf("vérifier la destination : %w", err)
	}
	if err := os.MkdirAll(absFolder, 0o755); err != nil {
		return domain.Note{}, fmt.Errorf("créer le dossier : %w", err)
	}
	return domain.Note{}, nil
}

// validateFolderRelPath vérifie qu'un chemin relatif de dossier est
// utilisable en interne : sous notes/, pas de traversée, pas d'absolu,
// pas vide. Différent de validateNoteRelPath (pas d'exigence d'extension).
func validateFolderRelPath(p string) error {
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(p)))
	if cleaned == "" || cleaned == "." || cleaned == "/" {
		return errors.New("chemin de dossier invalide")
	}
	if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") {
		return errors.New("chemin de dossier invalide")
	}
	if !strings.HasPrefix(cleaned, "notes/") {
		return errors.New("un dossier doit être rangé sous notes/")
	}
	if strings.HasSuffix(cleaned, "/") {
		return errors.New("un dossier ne doit pas finir par /")
	}
	return nil
}

// countFolderContents compte les notes directement présentes dans dirAbs
// (récursivement, à travers les sous-dossiers) et le nombre de
// sous-dossiers immédiats. Pour la modale de confirmation de suppression.
func countFolderContents(dirAbs string) (notes int, subdirs int, err error) {
	err = filepath.WalkDir(dirAbs, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if path == dirAbs {
			return nil
		}
		if d.IsDir() {
			if filepath.Dir(path) == dirAbs {
				subdirs++
			}
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) == ".md" {
			notes++
		}
		return nil
	})
	return notes, subdirs, err
}

// moveOrCopyDir renomme/déplace srcAbs vers dstAbs. Préfère os.Rename
// (atomique, rapide). Tombe en fallback sur un copy + remove quand
// src et dst sont sur des filesystems différents (os.Rename échoue
// alors avec EXDEV).
func moveOrCopyDir(srcAbs, dstAbs string) error {
	if err := os.Rename(srcAbs, dstAbs); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	if err := copyDir(srcAbs, dstAbs); err != nil {
		return err
	}
	return os.RemoveAll(srcAbs)
}

func copyDir(srcAbs, dstAbs string) error {
	if err := os.MkdirAll(dstAbs, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(srcAbs)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		src := filepath.Join(srcAbs, entry.Name())
		dst := filepath.Join(dstAbs, entry.Name())
		if entry.IsDir() {
			if err := copyDir(src, dst); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := writeAtomic(dst, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// MoveFolder déplace (ou renomme) un dossier sous notes/. Si newRel existe,
// l'opération échoue. Les notes contenues sont réindexées en série sous
// mutationMu : on supprime les anciens chemins et on insère les nouveaux.
// Le sous-dossier destination du parent est créé à la volée par os.Rename.
func (s *Service) MoveFolder(oldRel, newRel string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	oldRel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(oldRel)))
	newRel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(newRel)))
	if err := validateFolderRelPath(oldRel); err != nil {
		return err
	}
	if err := validateFolderRelPath(newRel); err != nil {
		return err
	}
	if oldRel == newRel {
		return nil
	}
	oldAbs := filepath.Join(s.root, oldRel)
	newAbs := filepath.Join(s.root, newRel)
	srcInfo, err := os.Stat(oldAbs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w : %s", ErrFolderNotFound, oldRel)
		}
		return fmt.Errorf("vérifier le dossier source : %w", err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("%w : %s", ErrFolderNotFound, oldRel)
	}
	// Empêche de déplacer un dossier dans lui-même ou sous lui-même :
	// on refuse toute cible qui commence par l'ancien chemin.
	if newRel == oldRel || strings.HasPrefix(newRel+"/", oldRel+"/") {
		return errors.New("impossible de déplacer un dossier dans lui-même")
	}
	if _, err := os.Stat(newAbs); err == nil {
		return fmt.Errorf("un dossier existe déjà à %s", newRel)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("vérifier la destination : %w", err)
	}
	// os.Rename exige que le parent de destination existe : on le crée
	// à la volée (cohérent avec MoveNote qui crée les dossiers manquants).
	newParentAbs := filepath.Dir(newAbs)
	if err := os.MkdirAll(newParentAbs, 0o755); err != nil {
		return fmt.Errorf("préparer le dossier parent : %w", err)
	}
	// Collecte des chemins de notes à réindexer AVANT le rename (la cible
	// des notes change d'emplacement disque après).
	type relocation struct {
		oldPath string // chemin relatif à s.root
		newPath string
	}
	var relocations []relocation
	if err := filepath.WalkDir(oldAbs, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}
		rel, relErr := filepath.Rel(s.root, path)
		if relErr != nil {
			return relErr
		}
		newPath := newRel + strings.TrimPrefix(filepath.ToSlash(rel), oldRel)
		relocations = append(relocations, relocation{
			oldPath: filepath.ToSlash(rel),
			newPath: newPath,
		})
		return nil
	}); err != nil {
		return fmt.Errorf("parcourir le dossier : %w", err)
	}
	if err := moveOrCopyDir(oldAbs, newAbs); err != nil {
		return fmt.Errorf("déplacer le dossier : %w", err)
	}
	s.markInternalWrite(newAbs)
	// Réindex : on supprime les anciens chemins puis on upsert les nouveaux
	// en s'appuyant sur la lecture disque (gère le chiffrement, le parseur,
	// les dates mtime). En cas d'échec, on remet au mieux l'ancien index en
	// place : les fichiers existent toujours, juste ailleurs sur le disque.
	for _, r := range relocations {
		if err := s.index.Delete(r.oldPath); err != nil && !errors.Is(err, ErrNotFound) {
			return fmt.Errorf("réindex après déplacement : %w", err)
		}
	}
	for _, r := range relocations {
		note, openErr := s.OpenNote(r.newPath)
		if openErr != nil {
			continue
		}
		if err := s.index.Upsert(note); err != nil {
			return fmt.Errorf("réindex après déplacement : %w", err)
		}
	}
	return nil
}

// RenameFolder renomme un dossier (change uniquement le dernier segment).
// Valide le nouveau nom via slug() pour rester cohérent avec CreateFolder.
func (s *Service) RenameFolder(rel, newName string) error {
	rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	if err := validateFolderRelPath(rel); err != nil {
		return err
	}
	trimmed := strings.TrimSpace(newName)
	if trimmed == "" || strings.ContainsAny(trimmed, "/\\") || strings.Contains(trimmed, "..") {
		return errors.New("nom de dossier invalide")
	}
	folderSlug := slug(trimmed)
	if folderSlug == "" || (folderSlug == "note" && !strings.EqualFold(trimmed, "note")) {
		return errors.New("nom de dossier invalide")
	}
	parent := filepath.ToSlash(filepath.Dir(rel))
	if parent == "." {
		parent = "notes"
	}
	newRel := filepath.ToSlash(filepath.Join(parent, folderSlug))
	if newRel == rel {
		return nil
	}
	return s.MoveFolder(rel, newRel)
}

// DeleteFolder supprime un dossier de notes/. Si force=false et que le
// dossier contient des notes ou des sous-dossiers, renvoie ErrFolderNotEmpty.
// Sinon, déplace le sous-arbre complet dans .trash/<yyyy-mm-dd>/<ts>-<basename>/,
// retire les notes du dossier de l'index, et laisse les fichiers sur disque.
func (s *Service) DeleteFolder(rel string, force bool) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	if err := validateFolderRelPath(rel); err != nil {
		return err
	}
	abs := filepath.Join(s.root, rel)
	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w : %s", ErrFolderNotFound, rel)
		}
		return fmt.Errorf("vérifier le dossier : %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w : %s", ErrFolderNotFound, rel)
	}
	if !force {
		notes, subdirs, countErr := countFolderContents(abs)
		if countErr != nil {
			return fmt.Errorf("compter le contenu : %w", countErr)
		}
		if notes > 0 || subdirs > 0 {
			return ErrFolderNotEmpty
		}
	}
	// Réindex : on retire toutes les notes du dossier de l'index avant
	// le rename. On collecte d'abord les chemins, on agira sur l'index
	// uniquement si le déplacement réussit.
	relocated := make([]string, 0)
	notePaths := make([]string, 0)
	if err := filepath.WalkDir(abs, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}
		relPath, relErr := filepath.Rel(s.root, path)
		if relErr != nil {
			return relErr
		}
		notePaths = append(notePaths, filepath.ToSlash(relPath))
		return nil
	}); err != nil {
		return fmt.Errorf("parcourir le dossier : %w", err)
	}
	now := time.Now().UTC()
	day := now.Format("2006-01-02")
	trashDir := filepath.Join(s.root, ".trash", day)
	if err := os.MkdirAll(trashDir, 0o755); err != nil {
		return fmt.Errorf("préparer la corbeille : %w", err)
	}
	dest := filepath.Join(trashDir, fmt.Sprintf("%s-%s", now.Format("20060102T150405.000"), filepath.Base(rel)))
	// Si par malchance deux suppressions tombent dans la même milliseconde,
	// on suffixe. Le contenu d'origine reste intouché.
	if _, err := os.Stat(dest); err == nil {
		dest = filepath.Join(trashDir, fmt.Sprintf("%s-%s-%d", now.Format("20060102T150405.000"), filepath.Base(rel), len(relocated)+1))
	}
	if err := os.Rename(abs, dest); err != nil {
		return fmt.Errorf("déplacer vers la corbeille : %w", err)
	}
	s.markInternalWrite(dest)
	relocated = append(relocated, dest)
	for _, p := range notePaths {
		if err := s.index.Delete(p); err != nil && !errors.Is(err, ErrNotFound) {
			return fmt.Errorf("retirer la note de l'index : %w", err)
		}
	}
	return nil
}

// FolderContents retourne le nombre de notes et de sous-dossiers contenus
// directement ou indirectement dans le dossier. Sert au frontend pour
// afficher l'avertissement "Supprimer N notes et M sous-dossiers" avant
// une suppression forcée.
func (s *Service) FolderContents(rel string) (FolderContentsInfo, error) {
	if err := s.requireUnlocked(); err != nil {
		return FolderContentsInfo{}, err
	}
	rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	if err := validateFolderRelPath(rel); err != nil {
		return FolderContentsInfo{}, err
	}
	abs := filepath.Join(s.root, rel)
	info, statErr := os.Stat(abs)
	if statErr != nil {
		if errors.Is(statErr, fs.ErrNotExist) {
			return FolderContentsInfo{}, fmt.Errorf("%w : %s", ErrFolderNotFound, rel)
		}
		return FolderContentsInfo{}, statErr
	}
	if !info.IsDir() {
		return FolderContentsInfo{}, fmt.Errorf("%w : %s", ErrFolderNotFound, rel)
	}
	notes, subs, countErr := countFolderContents(abs)
	if countErr != nil {
		return FolderContentsInfo{}, countErr
	}
	return FolderContentsInfo{Notes: notes, Subdirs: subs}, nil
}

// resolveNoteParent normalise un chemin parent fourni par l'API. Une chaîne
// vide est résolue vers defaultParent. Tout chemin fourni doit commencer
// par "notes/" (ou être vide) : on refuse ce qui pointe hors de la zone
// des notes (assets/, templates/, etc.) pour respecter la règle "tout
// reste sous notes/". Le résultat est un chemin prêt à être combiné
// avec un nom de fichier via filepath.Join.
func (s *Service) resolveNoteParent(parentRelPath, defaultParent string) (string, error) {
	if parentRelPath == "" {
		return defaultParent, nil
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(parentRelPath)))
	if cleaned == "." || cleaned == "/" {
		return defaultParent, nil
	}
	if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") {
		return "", errors.New("chemin de dossier invalide")
	}
	if !strings.HasPrefix(cleaned, "notes/") {
		return "", errors.New("un dossier doit être rangé sous notes/")
	}
	return cleaned, nil
}

// resolveTemplateBody retourne le contenu d'un template : d'abord le
// TemplateLoader, puis fallback sur la fonction template() historique
// pour les clés built-in (meeting, daily, blank).
func (s *Service) resolveTemplateBody(key string) string {
	if s.templates != nil && key != "" {
		if t, err := s.templates.Get(key); err == nil {
			return t.Body
		}
	}
	return template(key)
}

func (s *Service) SaveNote(note domain.Note) (domain.Note, error) {
	s.mutationMu.RLock()
	defer s.mutationMu.RUnlock()
	if err := s.requireUnlocked(); err != nil {
		return domain.Note{}, err
	}
	note.RelativePath = filepath.ToSlash(filepath.Clean(filepath.FromSlash(note.RelativePath)))
	path, err := s.absoluteNotePath(note.RelativePath)
	if err != nil {
		return domain.Note{}, err
	}
	now := nowUTC()
	if note.CreatedAt.IsZero() {
		note.CreatedAt = now
	}
	note.UpdatedAt = now
	note.Title = strings.TrimSpace(note.Title)
	if note.Title == "" {
		note.Title = "Sans titre"
	}

	// Snapshot d'historique avant écrasement (best-effort).
	cfg, _ := s.config.Load()
	maxVersions := config.Default().HistoryPerNote
	if cfg.HistoryPerNote > 0 {
		maxVersions = cfg.HistoryPerNote
	}
	if maxVersions > 0 {
		if _, err := os.Stat(path); err == nil {
			_, _ = s.snapshotHistory(note.RelativePath, maxVersions)
		}
	}

	if err := s.writePayload(note.RelativePath, []byte(serialize(note)), 0o644); err != nil {
		return domain.Note{}, err
	}
	s.markInternalWrite(path)
	if err := s.index.Upsert(note); err != nil {
		return domain.Note{}, fmt.Errorf("mettre à jour l'index : %w", err)
	}
	return note, nil
}

// DeleteNote fait un soft-delete : la note est déplacée vers la corbeille.
func (s *Service) DeleteNote(relativePath string) error {
	s.mutationMu.RLock()
	defer s.mutationMu.RUnlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	path, err := s.absoluteNotePath(relativePath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Synchroniser l'index si le fichier a disparu hors app.
			_ = s.index.Delete(relativePath)
			return nil
		}
		return fmt.Errorf("vérifier la note : %w", err)
	}
	if _, err := softDelete(s.root, path); err != nil {
		return err
	}
	return s.index.Delete(relativePath)
}

// ListTrash retourne les entrées actuellement en corbeille.
func (s *Service) ListTrash() ([]TrashEntry, error) {
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	return listTrash(s.root)
}

// RestoreFromTrash remet en place une note précédemment supprimée.
func (s *Service) RestoreFromTrash(id string) (domain.Note, error) {
	s.mutationMu.RLock()
	defer s.mutationMu.RUnlock()
	if err := s.requireUnlocked(); err != nil {
		return domain.Note{}, err
	}
	entries, err := listTrash(s.root)
	if err != nil {
		return domain.Note{}, err
	}
	var target *TrashEntry
	for i, e := range entries {
		if e.ID == id {
			target = &entries[i]
			break
		}
	}
	if target == nil {
		return domain.Note{}, fmt.Errorf("entrée de corbeille introuvable : %s", id)
	}
	originalPath, err := restoreFromTrash(s.root, *target)
	if err != nil {
		return domain.Note{}, err
	}
	s.markInternalWrite(filepath.Join(s.root, filepath.FromSlash(originalPath)))
	note, err := s.OpenNote(originalPath)
	if err != nil {
		return domain.Note{}, err
	}
	if err := s.index.Upsert(note); err != nil {
		return domain.Note{}, err
	}
	return note, nil
}

// EmptyTrash vide la corbeille.
func (s *Service) EmptyTrash() error {
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	entries, err := listTrash(s.root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		_ = os.Remove(e.TrashPath)
		_ = os.Remove(e.TrashPath + ".meta")
	}
	return nil
}

// GetConfig retourne la configuration courante.
func (s *Service) GetConfig() (config.Config, error) {
	return s.config.Load()
}

// UpdateConfig enregistre la configuration.
func (s *Service) UpdateConfig(cfg config.Config) error {
	return s.config.Save(cfg)
}

// OpenDailyNote ouvre (ou crée) la note du jour.
// La création est inconditionnelle : le clic sur l'icône calendrier doit
// toujours produire une note, indépendamment de la config AutoDailyNote
// (qui ne concerne que l'auto-création au démarrage).
func (s *Service) OpenDailyNote() (domain.Note, error) {
	rel, err := s.ensureDailyNoteImpl()
	if err != nil {
		return domain.Note{}, err
	}
	return s.OpenNote(rel)
}

// ListTemplates retourne la liste des templates disponibles (built-in + user).
func (s *Service) ListTemplates() []Template {
	return s.templates.List()
}

// GetTemplate retourne un template par ID.
func (s *Service) GetTemplate(id string) (Template, error) {
	return s.templates.Get(id)
}

// MoveNote déplace (ou renomme) une note vers un nouveau chemin relatif.
// Les dossiers manquants sont créés. Si newPath existe déjà, l'opération
// échoue. Le contenu est déplacé atomiquement (os.Rename quand possible,
// sinon copy + delete).
func (s *Service) MoveNote(oldPath, newPath string) (domain.Note, error) {
	s.mutationMu.RLock()
	defer s.mutationMu.RUnlock()
	if err := s.requireUnlocked(); err != nil {
		return domain.Note{}, err
	}
	oldPath = filepath.ToSlash(filepath.Clean(oldPath))
	newPath = filepath.ToSlash(filepath.Clean(newPath))
	if err := s.validateNoteRelPath(newPath); err != nil {
		return domain.Note{}, err
	}
	if err := s.validateNoteRelPath(oldPath); err != nil {
		return domain.Note{}, err
	}
	if oldPath == newPath {
		return s.OpenNote(oldPath)
	}
	wasPinned, _ := s.index.IsPinned(oldPath)
	src := filepath.Join(s.root, filepath.FromSlash(oldPath))
	dst := filepath.Join(s.root, filepath.FromSlash(newPath))
	if _, err := os.Stat(src); err != nil {
		return domain.Note{}, fmt.Errorf("note source introuvable : %s", oldPath)
	}
	if _, err := os.Stat(dst); err == nil {
		return domain.Note{}, fmt.Errorf("un fichier existe déjà à %s", newPath)
	}
	raw, err := s.readPayload(oldPath)
	if err != nil {
		return domain.Note{}, err
	}
	if err := s.writePayload(newPath, raw, 0o644); err != nil {
		return domain.Note{}, fmt.Errorf("déplacer : %w", err)
	}
	s.markInternalWrite(dst)
	if err := os.Remove(src); err != nil {
		_ = os.Remove(dst)
		return domain.Note{}, fmt.Errorf("supprimer l'ancienne note : %w", err)
	}
	// Réindexe : supprimer l'ancien, upsert le nouveau.
	if err := s.index.Delete(oldPath); err != nil && !errors.Is(err, ErrNotFound) {
		return domain.Note{}, err
	}
	note, err := s.OpenNote(newPath)
	if err != nil {
		return domain.Note{}, err
	}
	if err := s.index.Upsert(note); err != nil {
		return domain.Note{}, err
	}
	if wasPinned {
		if err := s.index.Pin(newPath, true); err != nil {
			return domain.Note{}, err
		}
	}
	return note, nil
}

// DuplicateNote crée une copie d'une note vers un nouveau chemin.
// Le nom de fichier est suffixé par "-copie" si la cible existe déjà.
// Les tags et la date de création sont remis à zéro.
func (s *Service) DuplicateNote(relativePath string) (domain.Note, error) {
	if err := s.requireUnlocked(); err != nil {
		return domain.Note{}, err
	}
	src := relativePath
	if err := s.validateNoteRelPath(src); err != nil {
		return domain.Note{}, err
	}
	note, err := s.OpenNote(src)
	if err != nil {
		return domain.Note{}, err
	}
	dir := filepath.Dir(src)
	ext := filepath.Ext(src)
	base := strings.TrimSuffix(filepath.Base(src), ext)
	newBase := base + "-copie"
	dst := filepath.ToSlash(filepath.Join(dir, newBase+ext))
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(s.root, filepath.FromSlash(dst))); err != nil {
			break
		}
		dst = filepath.ToSlash(filepath.Join(dir, fmt.Sprintf("%s-copie-%d%s", base, i, ext)))
	}
	now := nowUTC()
	note.RelativePath = dst
	note.Title = strings.TrimSpace(note.Title)
	if note.Title != "" {
		note.Title = note.Title + " (copie)"
	}
	note.CreatedAt = now
	note.UpdatedAt = now
	return s.SaveNote(note)
}

// OpenInExplorer ouvre le fichier (ou son dossier) dans le gestionnaire
// de fichiers natif (Finder / Explorer / xdg-open).
// Sur les plateformes supportées par le runtime de Wails, l'OS
// est sélectionné automatiquement.
func (s *Service) OpenInExplorer(relativePath string, reveal bool) error {
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	abs, err := s.absoluteNotePath(relativePath)
	if err != nil {
		return err
	}
	target := abs
	if reveal {
		target = filepath.Dir(abs)
	}
	return openInOS(target)
}

// RenameTitle met à jour uniquement le titre de la note (sans toucher
// au chemin). Pratique pour le renommage inline.
func (s *Service) RenameTitle(relativePath, newTitle string) (domain.Note, error) {
	note, err := s.OpenNote(relativePath)
	if err != nil {
		return domain.Note{}, err
	}
	note.Title = strings.TrimSpace(newTitle)
	if note.Title == "" {
		note.Title = "Sans titre"
	}
	return s.SaveNote(note)
}

// GetBacklinks retourne les notes qui référencent le titre donné avec un
// wiki-lien [[Titre]]. Utilisé pour le panneau de backlinks de l'éditeur.
// excludePath est ignoré dans les résultats (la note courante ne se backlink
// pas elle-même).
func (s *Service) GetBacklinks(title, excludePath string, limit int) ([]domain.NoteSummary, error) {
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	return s.index.GetBacklinks(title, SearchOpts{Limit: limit, ExcludePath: excludePath})
}

// SaveAssetWithMaxSize applique une limite de taille explicite.
func (s *Service) SaveAssetWithMaxSize(data []byte, filename string, maxBytes int) (string, error) {
	if maxBytes > 0 && len(data) > maxBytes {
		return "", fmt.Errorf("asset trop volumineux : %d > %d octets", len(data), maxBytes)
	}
	return s.SaveAsset(data, filename)
}

// --- Phase 5 : thèmes, stats, recovery ------------------------------------

// ListThemes retourne tous les thèmes disponibles (built-in + utilisateur).
func (s *Service) ListThemes() []Theme {
	if s.themes == nil {
		return builtinThemes()
	}
	return s.themes.List()
}

// Theme retourne un thème par ID.
func (s *Service) Theme(id string) (Theme, error) {
	if s.themes == nil {
		for _, t := range builtinThemes() {
			if t.ID == id {
				return t, nil
			}
		}
		return Theme{}, fmt.Errorf("thème introuvable : %q", id)
	}
	return s.themes.Get(id)
}

// LoadState lit state.json (crash recovery + onboarding).
func (s *Service) LoadState() (StateFile, error) {
	if s.state == nil {
		return StateFile{}, nil
	}
	return s.state.Load()
}

// SaveState écrit state.json de manière atomique.
func (s *Service) SaveState(state StateFile) error {
	if s.state == nil {
		return fmt.Errorf("state store non initialisé")
	}
	return s.state.Save(state)
}

// MarkOnboardingCompleted persiste le drapeau d'onboarding et le snapshot
// des choix de l'utilisateur. L'argument onboarding peut être nil pour
// ne mettre à jour que le drapeau.
func (s *Service) MarkOnboardingCompleted(onboarding *Onboarding) error {
	state, err := s.LoadState()
	if err != nil {
		state = StateFile{}
	}
	state.OnboardingCompleted = true
	if onboarding != nil {
		onboarding.Skipped = false
		if onboarding.CompletedAt.IsZero() {
			onboarding.CompletedAt = nowUTC()
		}
		state.Onboarding = onboarding
	}
	return s.SaveState(state)
}

// SetDirtyBuffer enregistre un buffer en cours d'édition pour la
// récupération après crash. Si diskMTime est non nulle, on la conserve
// pour pouvoir comparer à la prochaine lecture.
func (s *Service) SetDirtyBuffer(notePath, buffer string, diskMTime time.Time) error {
	s.mutationMu.RLock()
	defer s.mutationMu.RUnlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	state, err := s.LoadState()
	if err != nil {
		state = StateFile{}
	}
	state.Dirty = true
	state.NotePath = notePath
	s.securityMu.RLock()
	encrypted := s.vaultState == VaultUnlocked
	key := append([]byte(nil), s.vaultKey...)
	s.securityMu.RUnlock()
	if encrypted {
		ciphertext, err := encryptFilePayload(key, ".notevault/state.json#buffer", []byte(buffer))
		zeroBytes(key)
		if err != nil {
			return err
		}
		state.Buffer = ""
		state.BufferCiphertext = base64.StdEncoding.EncodeToString(ciphertext)
	} else {
		zeroBytes(key)
		state.Buffer = buffer
		state.BufferCiphertext = ""
	}
	state.BufferSavedAt = nowUTC()
	if !diskMTime.IsZero() {
		t := diskMTime.UTC()
		state.DiskModifiedAt = &t
	}
	return s.SaveState(state)
}

// ClearDirtyBuffer efface le buffer en attente (après save réussi ou
// récupération rejetée).
func (s *Service) ClearDirtyBuffer() error {
	s.mutationMu.RLock()
	defer s.mutationMu.RUnlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	state, err := s.LoadState()
	if err != nil {
		return nil
	}
	if !state.Dirty && state.Buffer == "" {
		return nil
	}
	state.Dirty = false
	state.Buffer = ""
	state.BufferCiphertext = ""
	state.BufferSavedAt = time.Time{}
	state.NotePath = ""
	state.DiskModifiedAt = nil
	return s.SaveState(state)
}

// SnapshotForStartup combine l'état d'onboarding et la proposition de
// récupération. À appeler au démarrage du frontend pour décider de
// l'affichage initial.
func (s *Service) SnapshotForStartup() (RecoverySnapshot, error) {
	if err := s.requireUnlocked(); err != nil {
		return RecoverySnapshot{}, err
	}
	state, err := s.LoadState()
	if err != nil {
		return RecoverySnapshot{}, err
	}
	snap := RecoverySnapshot{
		Onboarding: state.Onboarding,
	}
	if state.Dirty && state.NotePath != "" {
		diskMTime, err := s.fileModified(state.NotePath)
		if err == nil {
			if ShouldOfferRecovery(state, diskMTime) {
				buffer := state.Buffer
				if state.BufferCiphertext != "" {
					encoded, decodeErr := base64.StdEncoding.DecodeString(state.BufferCiphertext)
					if decodeErr != nil {
						return RecoverySnapshot{}, ErrUnlockFailed
					}
					s.securityMu.RLock()
					key := append([]byte(nil), s.vaultKey...)
					s.securityMu.RUnlock()
					plain, openErr := decryptFilePayload(key, ".notevault/state.json#buffer", encoded)
					zeroBytes(key)
					if openErr != nil {
						return RecoverySnapshot{}, ErrUnlockFailed
					}
					buffer = string(plain)
				}
				snap.HasRecovery = true
				snap.NotePath = state.NotePath
				snap.Buffer = buffer
				snap.BufferSavedAt = state.BufferSavedAt
				if state.DiskModifiedAt != nil {
					snap.DiskModifiedAt = *state.DiskModifiedAt
				}
			}
		}
	}
	return snap, nil
}

func (s *Service) fileModified(relPath string) (time.Time, error) {
	abs, err := s.absoluteNotePath(relPath)
	if err != nil {
		return time.Time{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime().UTC(), nil
}

func (s *Service) validateNoteRelPath(p string) error {
	return validateNoteRelPath(p)
}

func (s *Service) absoluteNotePath(relativePath string) (string, error) {
	relativePath = filepath.Clean(filepath.FromSlash(relativePath))
	if relativePath == "." || filepath.IsAbs(relativePath) || strings.HasPrefix(relativePath, "..") {
		return "", errors.New("chemin de note invalide")
	}
	if filepath.Ext(relativePath) != ".md" {
		return "", errors.New("une note doit avoir l'extension .md")
	}
	if !strings.HasPrefix(filepath.ToSlash(relativePath), "notes/") {
		return "", errors.New("une note doit être rangée sous notes/")
	}
	return filepath.Join(s.root, relativePath), nil
}

func (s *Service) readAbsolute(path string) (domain.Note, error) {
	relativePath, err := filepath.Rel(s.root, path)
	if err != nil {
		return domain.Note{}, err
	}
	relativePath = filepath.ToSlash(relativePath)
	raw, err := s.readPayload(relativePath)
	if err != nil {
		return domain.Note{}, fmt.Errorf("lire la note : %w", err)
	}
	note := parse(string(raw))
	note.RelativePath = relativePath
	if note.Title == "" {
		note.Title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	info, err := os.Stat(path)
	if err == nil && note.UpdatedAt.IsZero() {
		note.UpdatedAt = info.ModTime().UTC()
	}
	return note, nil
}

func (s *Service) startWatcher() error {
	if !s.watcherWanted || s.watcher != nil {
		return nil
	}
	if err := s.requireUnlocked(); err != nil {
		return nil
	}
	s.indexCtx, s.indexStop = context.WithCancel(context.Background())
	w, err := NewWatcher(s.indexCtx, s.root, s.index, s.reindexFromPath)
	if err != nil {
		s.indexStop()
		s.indexStop = nil
		return err
	}
	s.watcher = w
	return nil
}

func (s *Service) stopWatcher() {
	w := s.watcher
	s.watcher = nil
	if s.indexStop != nil {
		s.indexStop()
		s.indexStop = nil
	}
	if w != nil {
		_ = w.Close()
	}
}
