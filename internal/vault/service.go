package vault

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/votre-compte/notevault/internal/config"
	"github.com/votre-compte/notevault/internal/domain"
)

// Service est la façade métier du coffre. Elle combine un index SQLite
// (cache requêtable), un Store de configuration et des opérations fichiers
// atomiques. Toute mutation doit passer par elle pour rester cohérente.
type Service struct {
	root      string
	index     Index
	config    *config.Store
	watcher   *Watcher
	templates *TemplateLoader
	indexCtx  context.Context
	indexStop context.CancelFunc
}

// Options configure la construction du service.
type Options struct {
	Root         string
	Index        Index         // si nil, un index SQLite est créé
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
	idx := opts.Index
	if idx == nil {
		idx, err = newSQLiteIndex(filepath.Join(root, ".notevault", "index.db"))
		if err != nil {
			return nil, err
		}
	}
	cfg := opts.Config
	if cfg == nil {
		cfg = config.NewStore(root)
	}
	s := &Service{
		root:      root,
		index:     idx,
		config:    cfg,
		templates: NewTemplateLoader(root),
	}
	// Purge de la corbeille au démarrage.
	if loaded, err := cfg.Load(); err == nil {
		_ = purgeTrash(root, loaded.TrashRetentionDays)
	} else {
		_ = purgeTrash(root, config.Default().TrashRetentionDays)
	}
	if opts.StartWatcher {
		s.indexCtx, s.indexStop = context.WithCancel(context.Background())
		w, err := NewWatcher(s.indexCtx, root, idx, s.reindexFromPath)
		if err != nil {
			idx.Close()
			return nil, err
		}
		s.watcher = w
	}
	return s, nil
}

// NewDefaultService crée le service avec les options par défaut.
// Lance l'indexation initiale en arrière-plan puis le watcher.
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
	if s.watcher != nil {
		_ = s.watcher.Close()
	}
	if s.indexStop != nil {
		s.indexStop()
	}
	if s.index != nil {
		return s.index.Close()
	}
	return nil
}

// IndexNow déclenche une réindexation complète du dossier notes/.
// Synchrone : utilisé au bootstrap si l'index est vide ou en cas de
// récupération après crash. Ne fait rien si l'index contient déjà des
// entrées (le watcher fsnotify prend le relais).
func (s *Service) IndexNow(ctx context.Context, reporter progressReporter) error {
	existing, err := s.index.List(ListFilter{Limit: 1})
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		if reporter != nil {
			reporter.OnProgress("index", 0, 0)
		}
		return nil
	}
	return IndexExisting(ctx, s.root, s.index, reporter)
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
		return
	}
	note, err := readAbsoluteFile(s.root, absPath)
	if err != nil {
		return
	}
	_ = s.index.Upsert(note)
}

func (s *Service) ListNotes() ([]domain.NoteSummary, error) {
	return s.ListNotesFiltered(FilterQuery{}, 5000)
}

// ListNotesFiltered applique une requête structurée et retourne les
// résumés de notes correspondants. Si la requête est vide, équivaut
// à ListNotes().
func (s *Service) ListNotesFiltered(q FilterQuery, limit int) ([]domain.NoteSummary, error) {
	if limit <= 0 {
		limit = 5000
	}
	summaries, err := s.index.List(q.ToListFilter(limit))
	if err != nil {
		return nil, fmt.Errorf("lister les notes : %w", err)
	}
	if q.IsEmpty() {
		// Tri stable déjà assuré par SQL.
		return summaries, nil
	}
	// Tri mémoire stable par updated_at DESC (le SQL fait déjà le tri).
	return summaries, nil
}

// ListPinned retourne les notes épinglées (par ordre d'épinglage).
func (s *Service) ListPinned() ([]domain.NoteSummary, error) {
	out, err := s.index.ListPinned()
	if err != nil {
		return nil, fmt.Errorf("lister les épinglées : %w", err)
	}
	return out, nil
}

// ListFolders retourne les dossiers connus du coffre pour la vue arbre.
func (s *Service) ListFolders() ([]FolderInfo, error) {
	return s.index.ListFolders()
}

// Pin épingle ou désépingle une note.
func (s *Service) Pin(relativePath string, pinned bool) error {
	if _, err := s.absoluteNotePath(relativePath); err != nil {
		return err
	}
	return s.index.Pin(relativePath, pinned)
}

// IsPinned retourne l'état d'épinglage d'une note.
func (s *Service) IsPinned(relativePath string) (bool, error) {
	return s.index.IsPinned(relativePath)
}

// ListTags retourne tous les tags avec leur compte.
func (s *Service) ListTags() ([]TagCount, error) {
	return s.index.ListTags()
}

// Search interroge l'index full-text.
func (s *Service) Search(query string, limit int) ([]domain.NoteSummary, error) {
	return s.index.Search(query, SearchOpts{Limit: limit})
}

func (s *Service) OpenNote(relativePath string) (domain.Note, error) {
	path, err := s.absoluteNotePath(relativePath)
	if err != nil {
		return domain.Note{}, err
	}
	return s.readAbsolute(path)
}

func (s *Service) CreateNote(title string, templateKey string) (domain.Note, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Nouvelle note"
	}

	now := nowUTC()
	filename := fmt.Sprintf("%s-%s.md", now.Format("20060102-150405"), slug(title))
	relativePath := filepath.ToSlash(filepath.Join("notes", "inbox", filename))
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

	if err := writeAtomic(path, []byte(serialize(note)), 0o644); err != nil {
		return domain.Note{}, err
	}
	if err := s.index.Upsert(note); err != nil {
		return domain.Note{}, fmt.Errorf("mettre à jour l'index : %w", err)
	}
	return note, nil
}

// DeleteNote fait un soft-delete : la note est déplacée vers la corbeille.
func (s *Service) DeleteNote(relativePath string) error {
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
	return listTrash(s.root)
}

// RestoreFromTrash remet en place une note précédemment supprimée.
func (s *Service) RestoreFromTrash(id string) (domain.Note, error) {
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
	src := filepath.Join(s.root, filepath.FromSlash(oldPath))
	dst := filepath.Join(s.root, filepath.FromSlash(newPath))
	if _, err := os.Stat(src); err != nil {
		return domain.Note{}, fmt.Errorf("note source introuvable : %s", oldPath)
	}
	if _, err := os.Stat(dst); err == nil {
		return domain.Note{}, fmt.Errorf("un fichier existe déjà à %s", newPath)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return domain.Note{}, fmt.Errorf("préparer le dossier destination : %w", err)
	}
	if err := os.Rename(src, dst); err != nil {
		// os.Rename échoue cross-device : fallback copy + delete.
		if cerr := copyFileAtomic(src, dst); cerr != nil {
			return domain.Note{}, fmt.Errorf("déplacer : %w", cerr)
		}
		_ = os.Remove(src)
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
	return note, nil
}

// DuplicateNote crée une copie d'une note vers un nouveau chemin.
// Le nom de fichier est suffixé par "-copie" si la cible existe déjà.
// Les tags et la date de création sont remis à zéro.
func (s *Service) DuplicateNote(relativePath string) (domain.Note, error) {
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

func (s *Service) validateNoteRelPath(p string) error {
	p = filepath.Clean(filepath.FromSlash(p))
	if p == "." || filepath.IsAbs(p) || strings.HasPrefix(p, "..") {
		return errors.New("chemin de note invalide")
	}
	if filepath.Ext(p) != ".md" {
		return errors.New("une note doit avoir l'extension .md")
	}
	if !strings.HasPrefix(filepath.ToSlash(p), "notes/") {
		return errors.New("une note doit être rangée sous notes/")
	}
	return nil
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
	raw, err := os.ReadFile(path)
	if err != nil {
		return domain.Note{}, fmt.Errorf("lire la note : %w", err)
	}
	relativePath, err := filepath.Rel(s.root, path)
	if err != nil {
		return domain.Note{}, err
	}
	note := parse(string(raw))
	note.RelativePath = filepath.ToSlash(relativePath)
	if note.Title == "" {
		note.Title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	info, err := os.Stat(path)
	if err == nil && note.UpdatedAt.IsZero() {
		note.UpdatedAt = info.ModTime().UTC()
	}
	return note, nil
}
