package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kvitrvn/notevault/internal/appconfig"
	"github.com/kvitrvn/notevault/internal/chat"
	"github.com/kvitrvn/notevault/internal/config"
	"github.com/kvitrvn/notevault/internal/domain"
	"github.com/kvitrvn/notevault/internal/updatecheck"
	"github.com/kvitrvn/notevault/internal/vault"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	ErrNoVaultOpen    = errors.New("aucun coffre ouvert")
	ErrVaultSwitching = errors.New("changement de coffre en cours")
)

type vaultSession struct {
	service   *vault.Service
	assetSrv  *vault.AssetServer
	assetPort int
	chatMu    sync.Mutex
	chat      *chat.Service
}

func (s *vaultSession) close() {
	if s == nil {
		return
	}
	if s.assetSrv != nil {
		_ = s.assetSrv.Stop()
	}
	s.chatMu.Lock()
	if s.chat != nil {
		s.chat.Close()
		s.chat = nil
	}
	s.chatMu.Unlock()
	if s.service != nil {
		_ = s.service.Close()
	}
}

func (s *vaultSession) chatService(secrets chat.SecretStore) (*chat.Service, error) {
	s.chatMu.Lock()
	defer s.chatMu.Unlock()
	if s.chat != nil {
		return s.chat, nil
	}
	service, err := chat.New(filepath.Join(s.service.Root(), ".notevault", "chat"), secrets)
	if err != nil {
		return nil, err
	}
	s.chat = service
	return service, nil
}

type appOptions struct {
	configPath     string
	legacyPath     string
	now            func() time.Time
	secrets        chat.SecretStore
	version        string
	checkForUpdate func(context.Context, string) (updatecheck.Result, error)
	pdfSaveDialog  func(context.Context, wailsruntime.SaveDialogOptions) (string, error)
	pdfBrowser     func() (detectedPDFBrowser, error)
	pdfExecutable  func() (string, error)
	pdfRender      func(context.Context, string, detectedPDFBrowser, vault.PDFDocument) ([]byte, error)
}

// App est la façade Wails. La session est absente tant qu'aucun coffre
// valide n'a été ouvert.
type App struct {
	ctx context.Context

	sessionMu sync.RWMutex
	session   *vaultSession
	switchMu  sync.Mutex
	switching atomic.Bool

	configMu sync.Mutex
	config   appconfig.Config
	store    *appconfig.Store
	secrets  chat.SecretStore

	now func() time.Time

	version        string
	checkForUpdate func(context.Context, string) (updatecheck.Result, error)
	pdfSaveDialog  func(context.Context, wailsruntime.SaveDialogOptions) (string, error)
	pdfBrowser     func() (detectedPDFBrowser, error)
	pdfExecutable  func() (string, error)
	pdfRender      func(context.Context, string, detectedPDFBrowser, vault.PDFDocument) ([]byte, error)
}

func NewApp() (*App, error) {
	configPath, err := appconfig.DefaultPath()
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("déterminer le dossier utilisateur : %w", err)
	}
	return newApp(appOptions{
		configPath: configPath,
		legacyPath: filepath.Join(home, "NoteVault"),
		now:        time.Now,
	})
}

func newApp(opts appOptions) (*App, error) {
	if opts.now == nil {
		opts.now = time.Now
	}
	if opts.secrets == nil {
		opts.secrets = chat.NewKeyringStore()
	}
	if opts.version == "" {
		opts.version = buildVersion
	}
	if opts.checkForUpdate == nil {
		checker := updatecheck.New(nil, "")
		opts.checkForUpdate = checker.Check
	}
	if opts.pdfSaveDialog == nil {
		opts.pdfSaveDialog = func(ctx context.Context, dialogOptions wailsruntime.SaveDialogOptions) (string, error) {
			return wailsruntime.SaveFileDialog(ctx, dialogOptions)
		}
	}
	if opts.pdfBrowser == nil {
		opts.pdfBrowser = detectPDFBrowser
	}
	if opts.pdfExecutable == nil {
		opts.pdfExecutable = os.Executable
	}
	if opts.pdfRender == nil {
		opts.pdfRender = renderPDFInWorker
	}
	store := appconfig.NewStore(opts.configPath)
	cfg, exists, err := store.Load()
	if err != nil {
		return nil, err
	}
	if !exists {
		cfg, err = migrateLegacyConfiguration(opts.legacyPath, opts.now())
		if err != nil {
			return nil, err
		}
		if err := store.Save(cfg); err != nil {
			return nil, err
		}
	}

	app := &App{
		config:         cfg,
		store:          store,
		now:            opts.now,
		secrets:        opts.secrets,
		version:        opts.version,
		checkForUpdate: opts.checkForUpdate,
		pdfSaveDialog:  opts.pdfSaveDialog,
		pdfBrowser:     opts.pdfBrowser,
		pdfExecutable:  opts.pdfExecutable,
		pdfRender:      opts.pdfRender,
	}
	if cfg.ActiveVault != "" {
		prepared, prepareErr := app.prepareSession(cfg.ActiveVault)
		if prepareErr == nil {
			app.session = prepared
		}
	}
	return app, nil
}

func migrateLegacyConfiguration(path string, now time.Time) (appconfig.Config, error) {
	cfg := appconfig.Default()
	statePath := filepath.Join(path, ".notevault", "state.json")
	if raw, err := os.ReadFile(statePath); err == nil {
		var state struct {
			OnboardingCompleted bool `json:"onboardingCompleted"`
		}
		if json.Unmarshal(raw, &state) == nil {
			cfg.OnboardingDismissed = state.OnboardingCompleted
		}
	}
	used, err := vault.LegacyVaultUsed(path)
	if err != nil {
		return cfg, fmt.Errorf("inspecter l’ancien coffre : %w", err)
	}
	if !used {
		return cfg, nil
	}
	canonical, err := vault.ValidateExistingVault(path)
	if err != nil {
		return cfg, nil
	}
	cfg.RecordOpen(canonical, now)
	return cfg, nil
}

func (a *App) Startup(ctx context.Context) { a.ctx = ctx }

func (a *App) Shutdown(context.Context) {
	a.switchMu.Lock()
	defer a.switchMu.Unlock()
	a.switching.Store(true)
	a.sessionMu.Lock()
	old := a.session
	a.session = nil
	a.sessionMu.Unlock()
	old.close()
}

func (a *App) acquireSession() (*vaultSession, func(), error) {
	if a.switching.Load() {
		return nil, nil, ErrVaultSwitching
	}
	a.sessionMu.RLock()
	if a.switching.Load() {
		a.sessionMu.RUnlock()
		return nil, nil, ErrVaultSwitching
	}
	if a.session == nil {
		a.sessionMu.RUnlock()
		return nil, nil, ErrNoVaultOpen
	}
	return a.session, a.sessionMu.RUnlock, nil
}

func withSession[T any](a *App, call func(*vaultSession) (T, error)) (T, error) {
	var zero T
	session, release, err := a.acquireSession()
	if err != nil {
		return zero, err
	}
	defer release()
	return call(session)
}

func (a *App) beginSwitch() (func(), error) {
	if !a.switchMu.TryLock() {
		return nil, ErrVaultSwitching
	}
	a.switching.Store(true)
	return func() {
		a.switching.Store(false)
		a.switchMu.Unlock()
	}, nil
}

func (a *App) prepareSession(path string) (*vaultSession, error) {
	canonical, err := vault.ValidateExistingVault(path)
	if err != nil {
		return nil, err
	}
	service, err := vault.New(vault.Options{Root: canonical, StartWatcher: true})
	if err != nil {
		return nil, fmt.Errorf("initialiser le coffre : %w", err)
	}
	if service.VaultStatus().State != vault.VaultLocked {
		if err := service.IndexNow(service.BootstrapContext(), nil); err != nil {
			_ = service.Close()
			return nil, fmt.Errorf("indexer le coffre : %w", err)
		}
	}
	assetSrv := vault.NewAssetServer(canonical)
	port, err := assetSrv.Start()
	if err != nil {
		_ = service.Close()
		return nil, fmt.Errorf("démarrer le serveur d’assets : %w", err)
	}
	return &vaultSession{service: service, assetSrv: assetSrv, assetPort: port}, nil
}

func (a *App) commitSession(prepared *vaultSession) error {
	a.sessionMu.Lock()

	a.configMu.Lock()
	next := a.config
	next.RecordOpen(prepared.service.Root(), a.now())
	if err := a.store.Save(next); err != nil {
		a.configMu.Unlock()
		a.sessionMu.Unlock()
		return err
	}
	a.config = next
	a.configMu.Unlock()

	old := a.session
	a.session = prepared
	a.sessionMu.Unlock()
	old.close()
	return nil
}

func (a *App) OpenVault(path string) error {
	finish, err := a.beginSwitch()
	if err != nil {
		return err
	}
	defer finish()
	prepared, err := a.prepareSession(path)
	if err != nil {
		return err
	}
	if err := a.commitSession(prepared); err != nil {
		prepared.close()
		return err
	}
	return nil
}

func (a *App) CreateVault(request domain.CreateVaultRequest) error {
	finish, err := a.beginSwitch()
	if err != nil {
		return err
	}
	defer finish()
	if err := vault.ValidateVaultName(request.Name); err != nil {
		return err
	}
	parent, err := vault.CanonicalExistingDir(request.ParentPath)
	if err != nil {
		return err
	}
	destination := filepath.Join(parent, request.Name)
	if _, err := os.Lstat(destination); err == nil {
		return errors.New("un fichier ou dossier porte déjà ce nom")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("vérifier la destination : %w", err)
	}
	tempRoot, err := os.MkdirTemp(parent, ".notevault-create-*")
	if err != nil {
		return fmt.Errorf("préparer le coffre temporaire : %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tempRoot)
		}
	}()
	service, err := vault.New(vault.Options{Root: tempRoot})
	if err != nil {
		return err
	}
	if request.Encrypted {
		if err := service.EnableEncryption(request.Passphrase); err != nil {
			_ = service.Close()
			return err
		}
	}
	if err := service.Close(); err != nil {
		return fmt.Errorf("finaliser le coffre : %w", err)
	}
	if err := os.Rename(tempRoot, destination); err != nil {
		return fmt.Errorf("installer le coffre : %w", err)
	}
	cleanup = false
	prepared, err := a.prepareSession(destination)
	if err != nil {
		return err
	}
	if err := a.commitSession(prepared); err != nil {
		prepared.close()
		return err
	}
	return nil
}

func (a *App) ApplicationStatus() (domain.ApplicationStatus, error) {
	a.configMu.Lock()
	cfg := a.config
	a.configMu.Unlock()

	a.sessionMu.RLock()
	activePath := ""
	mode := domain.ApplicationNoVault
	if a.session != nil {
		activePath = a.session.service.Root()
		state := a.session.service.VaultStatus().State
		if state == vault.VaultLocked || state == vault.VaultEnabling || state == vault.VaultDisabling {
			mode = domain.ApplicationLocked
		} else {
			mode = domain.ApplicationReady
		}
	}
	a.sessionMu.RUnlock()

	status := domain.ApplicationStatus{
		Mode:                mode,
		RecentVaults:        make([]domain.VaultInfo, 0, len(cfg.RecentVaults)),
		OnboardingDismissed: cfg.OnboardingDismissed,
		Version:             a.applicationVersion(),
	}
	for _, recent := range cfg.RecentVaults {
		info := vaultInfo(recent.Path, recent.LastOpenedAt, recent.Path == activePath)
		status.RecentVaults = append(status.RecentVaults, info)
		if info.Active {
			copyInfo := info
			status.ActiveVault = &copyInfo
		}
	}
	if activePath != "" && status.ActiveVault == nil {
		info := vaultInfo(activePath, time.Time{}, true)
		status.ActiveVault = &info
	}
	return status, nil
}

func (a *App) CheckForUpdates() (domain.UpdateStatus, error) {
	currentVersion := a.applicationVersion()
	status := domain.UpdateStatus{CurrentVersion: currentVersion}
	if currentVersion == "dev" {
		return status, nil
	}
	check := a.checkForUpdate
	if check == nil {
		checker := updatecheck.New(nil, "")
		check = checker.Check
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := check(ctx, currentVersion)
	if err != nil {
		return status, errors.New("impossible de vérifier les mises à jour")
	}
	return domain.UpdateStatus{
		CurrentVersion:  result.CurrentVersion,
		LatestVersion:   result.LatestVersion,
		UpdateAvailable: result.UpdateAvailable,
	}, nil
}

func (a *App) applicationVersion() string {
	if a.version == "" {
		return buildVersion
	}
	return a.version
}

func vaultInfo(path string, opened time.Time, active bool) domain.VaultInfo {
	available := false
	if _, err := vault.ValidateExistingVault(path); err == nil {
		available = true
	}
	_, encryptionErr := os.Stat(filepath.Join(path, ".notevault", "encryption.json"))
	return domain.VaultInfo{
		Name:         filepath.Base(path),
		Path:         path,
		Available:    available,
		Encrypted:    encryptionErr == nil,
		Active:       active,
		LastOpenedAt: opened,
	}
}

func (a *App) ForgetRecentVault(path string) error {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	next := a.config
	next.Forget(path)
	if err := a.store.Save(next); err != nil {
		return err
	}
	a.config = next
	return nil
}

func (a *App) SetOnboardingDismissed(dismissed bool) error {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	next := a.config
	next.OnboardingDismissed = dismissed
	if err := a.store.Save(next); err != nil {
		return err
	}
	a.config = next
	return nil
}

func (a *App) SelectExistingVaultDirectory() (string, error) {
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Ouvrir un coffre NoteVault existant",
	})
}

func (a *App) SelectVaultParentDirectory() (string, error) {
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Choisir l’emplacement du nouveau coffre",
	})
}

func (a *App) VaultPath() (string, error) {
	return withSession(a, func(s *vaultSession) (string, error) { return s.service.Root(), nil })
}

func (a *App) VaultStatus() (vault.VaultStatusInfo, error) {
	return withSession(a, func(s *vaultSession) (vault.VaultStatusInfo, error) { return s.service.VaultStatus(), nil })
}

func (a *App) EnableEncryption(passphrase string) error {
	return a.sessionError(func(s *vault.Service) error { return s.EnableEncryption(passphrase) })
}
func (a *App) UnlockVault(passphrase string) error {
	return a.sessionError(func(s *vault.Service) error { return s.UnlockVault(passphrase) })
}
func (a *App) ChangePassphrase(current, replacement string) error {
	return a.sessionError(func(s *vault.Service) error { return s.ChangePassphrase(current, replacement) })
}
func (a *App) DisableEncryption(passphrase string) error {
	return a.sessionError(func(s *vault.Service) error { return s.DisableEncryption(passphrase) })
}

func (a *App) sessionError(call func(*vault.Service) error) error {
	_, err := withSession(a, func(s *vaultSession) (struct{}, error) { return struct{}{}, call(s.service) })
	return err
}

func (a *App) ListNotes() ([]domain.NoteSummary, error) {
	return withSession(a, func(s *vaultSession) ([]domain.NoteSummary, error) { return s.service.ListNotes() })
}
func (a *App) ListNotesFiltered(filter vault.FilterQuery, limit int) ([]domain.NoteSummary, error) {
	if limit <= 0 {
		limit = 1000
	}
	return withSession(a, func(s *vaultSession) ([]domain.NoteSummary, error) { return s.service.ListNotesFiltered(filter, limit) })
}
func (a *App) ListPinned() ([]domain.NoteSummary, error) {
	return withSession(a, func(s *vaultSession) ([]domain.NoteSummary, error) { return s.service.ListPinned() })
}
func (a *App) ListFolders() ([]vault.FolderInfo, error) {
	return withSession(a, func(s *vaultSession) ([]vault.FolderInfo, error) { return s.service.ListFolders() })
}
func (a *App) PinNote(path string, pinned bool) error {
	return a.sessionError(func(s *vault.Service) error { return s.Pin(path, pinned) })
}
func (a *App) IsNotePinned(path string) (bool, error) {
	return withSession(a, func(s *vaultSession) (bool, error) { return s.service.IsPinned(path) })
}
func (a *App) ListTags() ([]vault.TagCount, error) {
	return withSession(a, func(s *vaultSession) ([]vault.TagCount, error) { return s.service.ListTags() })
}
func (a *App) SearchNotes(query string, limit int) ([]domain.NoteSummary, error) {
	if limit <= 0 {
		limit = 200
	}
	return withSession(a, func(s *vaultSession) ([]domain.NoteSummary, error) { return s.service.Search(query, limit) })
}
func (a *App) OpenDailyNote() (domain.Note, error) {
	return withSession(a, func(s *vaultSession) (domain.Note, error) { return s.service.OpenDailyNote() })
}
func (a *App) EnsureDailyNote() (string, error) {
	return withSession(a, func(s *vaultSession) (string, error) { return s.service.EnsureDailyNote() })
}
func (a *App) ListTemplates() []vault.Template {
	s, release, err := a.acquireSession()
	if err != nil {
		return nil
	}
	defer release()
	return s.service.ListTemplates()
}
func (a *App) CreateNoteFromTemplate(title, templateID string) (domain.Note, error) {
	return withSession(a, func(s *vaultSession) (domain.Note, error) { return s.service.CreateNote("", title, templateID) })
}
func (a *App) MoveNote(oldPath, newPath string) (domain.Note, error) {
	return withSession(a, func(s *vaultSession) (domain.Note, error) { return s.service.MoveNote(oldPath, newPath) })
}
func (a *App) DuplicateNote(path string) (domain.Note, error) {
	return withSession(a, func(s *vaultSession) (domain.Note, error) { return s.service.DuplicateNote(path) })
}
func (a *App) OpenInExplorer(path string, reveal bool) error {
	return a.sessionError(func(s *vault.Service) error { return s.OpenInExplorer(path, reveal) })
}
func (a *App) RenameTitle(path, title string) (domain.Note, error) {
	return withSession(a, func(s *vaultSession) (domain.Note, error) { return s.service.RenameTitle(path, title) })
}
func (a *App) GetBacklinks(title, exclude string, limit int) ([]domain.NoteSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	return withSession(a, func(s *vaultSession) ([]domain.NoteSummary, error) {
		return s.service.GetBacklinks(title, exclude, limit)
	})
}
func (a *App) SaveAsset(data []byte, filename string) (string, error) {
	return withSession(a, func(s *vaultSession) (string, error) { return s.service.SaveAsset(data, filename) })
}
func (a *App) ImportAssetFromFilePath(path string) (string, error) {
	return withSession(a, func(s *vaultSession) (string, error) { return s.service.ImportAssetFromFilePath(path) })
}
func (a *App) ListHistory(path string) ([]vault.HistoryEntry, error) {
	return withSession(a, func(s *vaultSession) ([]vault.HistoryEntry, error) { return s.service.ListHistory(path) })
}
func (a *App) ReadHistoryVersion(path, id string) (string, error) {
	return withSession(a, func(s *vaultSession) (string, error) { return s.service.ReadHistoryVersion(path, id) })
}
func (a *App) RestoreFromHistory(path, id string) (domain.Note, error) {
	return withSession(a, func(s *vaultSession) (domain.Note, error) { return s.service.RestoreFromHistory(path, id) })
}
func (a *App) DiffHistory(path, aID, bID string) (string, error) {
	return withSession(a, func(s *vaultSession) (string, error) { return s.service.DiffHistory(path, aID, bID) })
}
func (a *App) OpenAsset(path string) (string, error) {
	return withSession(a, func(s *vaultSession) (string, error) { return s.service.ResolveAsset(path) })
}
func (a *App) AssetURL(path string) (string, error) {
	return withSession(a, func(s *vaultSession) (string, error) {
		abs, err := s.service.ResolveAsset(path)
		if err != nil {
			return "", err
		}
		rel, err := filepath.Rel(s.service.Root(), abs)
		if err != nil {
			return "", fmt.Errorf("construire l’URL de l’asset : %w", err)
		}
		q := url.Values{}
		if s.assetSrv != nil {
			q.Set("t", s.assetSrv.Token())
		}
		u := url.URL{
			Scheme:   "http",
			Host:     fmt.Sprintf("127.0.0.1:%d", s.assetPort),
			Path:     "/files/" + filepath.ToSlash(rel),
			RawQuery: q.Encode(),
		}
		return u.String(), nil
	})
}
func (a *App) OpenNote(path string) (domain.Note, error) {
	return withSession(a, func(s *vaultSession) (domain.Note, error) { return s.service.OpenNote(path) })
}
func (a *App) CreateNote(parentRelPath, title, key string) (domain.Note, error) {
	return withSession(a, func(s *vaultSession) (domain.Note, error) { return s.service.CreateNote(parentRelPath, title, key) })
}

// CreateFolder crée un sous-dossier vide dans le coffre. parentRelPath est
// un chemin relatif sous notes/ (ou vide pour la racine). name est nettoyé
// côté backend (slug) et doit être non vide.
func (a *App) CreateFolder(parentRelPath, name string) (domain.Note, error) {
	return withSession(a, func(s *vaultSession) (domain.Note, error) { return s.service.CreateFolder(parentRelPath, name) })
}

// MoveFolder déplace (ou renomme) un dossier sous notes/. oldRel et
// newRel sont des chemins relatifs préfixés par "notes/". Le dossier
// cible ne doit pas déjà exister ; on refuse aussi de déplacer un
// dossier dans lui-même ou dans un de ses descendants.
func (a *App) MoveFolder(oldRel, newRel string) error {
	return a.sessionError(func(s *vault.Service) error { return s.MoveFolder(oldRel, newRel) })
}

// RenameFolder renomme uniquement le dernier segment d'un dossier.
// newName est slugifié côté backend pour rester cohérent avec CreateFolder.
func (a *App) RenameFolder(rel, newName string) error {
	return a.sessionError(func(s *vault.Service) error { return s.RenameFolder(rel, newName) })
}

// DeleteFolder supprime un dossier. Si force=false et que le dossier
// contient des notes ou des sous-dossiers, renvoie vault.ErrFolderNotEmpty
// que le frontend traduit en modale de confirmation. Avec force=true,
// le sous-arbre complet est déplacé dans la corbeille et les notes
// quittent l'index.
func (a *App) DeleteFolder(rel string, force bool) error {
	return a.sessionError(func(s *vault.Service) error { return s.DeleteFolder(rel, force) })
}

// FolderContents retourne le nombre de notes et de sous-dossiers contenus
// directement ou indirectement dans le dossier. Sert au frontend pour
// afficher l'avertissement "Supprimer N notes et M sous-dossiers" avant
// une suppression forcée.
func (a *App) FolderContents(rel string) (vault.FolderContentsInfo, error) {
	return withSession(a, func(s *vaultSession) (vault.FolderContentsInfo, error) { return s.service.FolderContents(rel) })
}
func (a *App) SaveNote(note domain.Note) (domain.Note, error) {
	return withSession(a, func(s *vaultSession) (domain.Note, error) { return s.service.SaveNote(note) })
}
func (a *App) DeleteNote(path string) error {
	return a.sessionError(func(s *vault.Service) error { return s.DeleteNote(path) })
}
func (a *App) ListTrash() ([]vault.TrashEntry, error) {
	return withSession(a, func(s *vaultSession) ([]vault.TrashEntry, error) { return s.service.ListTrash() })
}
func (a *App) RestoreFromTrash(id string) (domain.Note, error) {
	return withSession(a, func(s *vaultSession) (domain.Note, error) { return s.service.RestoreFromTrash(id) })
}
func (a *App) EmptyTrash() error {
	return a.sessionError(func(s *vault.Service) error { return s.EmptyTrash() })
}
func (a *App) GetConfig() (config.Config, error) {
	return withSession(a, func(s *vaultSession) (config.Config, error) { return s.service.GetConfig() })
}
func (a *App) UpdateConfig(cfg config.Config) error {
	return a.sessionError(func(s *vault.Service) error { return s.UpdateConfig(cfg) })
}
func (a *App) ListThemes() []vault.Theme {
	s, release, err := a.acquireSession()
	if err != nil {
		return nil
	}
	defer release()
	return s.service.ListThemes()
}
func (a *App) Theme(id string) (vault.Theme, error) {
	return withSession(a, func(s *vaultSession) (vault.Theme, error) { return s.service.Theme(id) })
}
func (a *App) ExportNotes(paths []string, dest string) error {
	return a.sessionError(func(s *vault.Service) error { return s.ExportNotes(paths, dest) })
}

// PDFExportOptions reports the local renderer availability and declarative
// themes. Browser discovery is read-only and never installs software.
func (a *App) PDFExportOptions() (vault.PDFExportOptionsInfo, error) {
	return withSession(a, func(session *vaultSession) (vault.PDFExportOptionsInfo, error) {
		themes, warnings := session.service.ListPDFThemes()
		info := vault.PDFExportOptionsInfo{Themes: themes, Warnings: warnings}
		if runtime.GOOS != "linux" {
			info.UnavailableReason = "L’export PDF est disponible uniquement sous Linux dans cette version."
			return info, nil
		}
		browser, err := a.pdfBrowser()
		if err != nil {
			info.UnavailableReason = "Installez Chromium ou Google Chrome, puis relancez NoteVault."
			return info, nil
		}
		info.Available = true
		info.Browser = browser.Name
		return info, nil
	})
}

// ExportNotePDF exports exactly one note. The destination comes exclusively
// from the native save dialog; the frontend cannot provide an arbitrary path.
func (a *App) ExportNotePDF(relativePath, themeID string, plaintextConfirmed bool) (string, error) {
	return withSession(a, func(session *vaultSession) (string, error) {
		if runtime.GOOS != "linux" {
			return "", errors.New("l’export PDF est disponible uniquement sous Linux")
		}
		if session.service.VaultStatus().EncryptionEnabled && !plaintextConfirmed {
			return "", errors.New("confirmez que le PDF contiendra la note en clair")
		}
		browser, err := a.pdfBrowser()
		if err != nil {
			return "", errors.New("installez Chromium ou Google Chrome pour exporter en PDF")
		}
		note, err := session.service.OpenNote(relativePath)
		if err != nil {
			return "", err
		}
		defaultName := strings.TrimSuffix(filepath.Base(note.RelativePath), filepath.Ext(note.RelativePath)) + ".pdf"
		ctx := a.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		destination, err := a.pdfSaveDialog(ctx, wailsruntime.SaveDialogOptions{
			Title:                "Exporter la note en PDF",
			DefaultFilename:      defaultName,
			CanCreateDirectories: true,
			Filters: []wailsruntime.FileFilter{
				{DisplayName: "Document PDF (*.pdf)", Pattern: "*.pdf"},
			},
		})
		if err != nil {
			return "", fmt.Errorf("ouvrir le dialogue d’export : %w", err)
		}
		if destination == "" {
			return "", nil
		}
		if !strings.EqualFold(filepath.Ext(destination), ".pdf") {
			destination += ".pdf"
		}
		document, err := session.service.BuildNotePDFDocument(relativePath, themeID, plaintextConfirmed)
		if err != nil {
			return "", err
		}
		executable, err := a.pdfExecutable()
		if err != nil {
			return "", fmt.Errorf("localiser NoteVault : %w", err)
		}
		renderContext, cancel := context.WithTimeout(ctx, pdfParentTimeout)
		defer cancel()
		pdf, err := a.pdfRender(renderContext, executable, browser, document)
		if err != nil {
			return "", err
		}
		if err := vault.WritePDFAtomic(destination, pdf); err != nil {
			return "", fmt.Errorf("écrire le PDF : %w", err)
		}
		return destination, nil
	})
}

func (a *App) Stats() (vault.Stats, error) {
	return withSession(a, func(s *vaultSession) (vault.Stats, error) { return s.service.Stats() })
}
func (a *App) SnapshotForStartup() (vault.RecoverySnapshot, error) {
	return withSession(a, func(s *vaultSession) (vault.RecoverySnapshot, error) { return s.service.SnapshotForStartup() })
}
func (a *App) MarkOnboardingCompleted(onboarding *vault.Onboarding) error {
	return a.sessionError(func(s *vault.Service) error { return s.MarkOnboardingCompleted(onboarding) })
}
func (a *App) SetDirtyBuffer(path, buffer string, mtime time.Time) error {
	return a.sessionError(func(s *vault.Service) error { return s.SetDirtyBuffer(path, buffer, mtime) })
}
func (a *App) ClearDirtyBuffer() error {
	return a.sessionError(func(s *vault.Service) error { return s.ClearDirtyBuffer() })
}

// PrepareChat récupère localement les passages pertinents, puis construit un
// aperçu anonymisé. Aucun appel au fournisseur de modèle n'est effectué ici.
func (a *App) PrepareChat(request chat.PrepareRequest) (chat.Preview, error) {
	s, release, err := a.acquireSession()
	if err != nil {
		return chat.Preview{}, err
	}
	if s.service.VaultStatus().EncryptionEnabled {
		release()
		return chat.Preview{}, chat.ErrEncryptedVault
	}
	notes := make([]chat.Note, 0, len(request.NotePaths))
	for _, path := range request.NotePaths {
		note, openErr := s.service.OpenNote(path)
		if openErr != nil {
			release()
			return chat.Preview{}, fmt.Errorf("ouvrir une note sélectionnée : %w", openErr)
		}
		notes = append(notes, chat.Note{Path: note.RelativePath, Title: note.Title, Content: note.Content})
	}
	service, err := s.chatService(a.secrets)
	release()
	if err != nil {
		return chat.Preview{}, err
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return service.Prepare(ctx, request, notes)
}

// SendPreparedChat envoie uniquement la charge anonymisée précédemment
// prévisualisée. Une clé ponctuelle prime sur celle du trousseau et n'est pas
// conservée par le service.
func (a *App) SendPreparedChat(request chat.SendRequest) (chat.Response, error) {
	s, release, err := a.acquireSession()
	if err != nil {
		return chat.Response{}, err
	}
	if s.service.VaultStatus().EncryptionEnabled {
		release()
		return chat.Response{}, chat.ErrEncryptedVault
	}
	service, err := s.chatService(a.secrets)
	release()
	if err != nil {
		return chat.Response{}, err
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return service.Send(ctx, request)
}

// GetChatSettings retourne uniquement les préférences non sensibles et la
// présence éventuelle de clés. Les secrets ne quittent jamais le backend.
func (a *App) GetChatSettings() (chat.Settings, error) {
	a.configMu.Lock()
	cfg := a.config
	a.configMu.Unlock()

	settings := chat.Settings{
		Provider:            chat.Provider(cfg.ChatProvider),
		Models:              make(map[chat.Provider]string, len(cfg.ChatModels)),
		ProvidersWithAPIKey: []chat.Provider{},
	}
	for provider, model := range cfg.ChatModels {
		settings.Models[chat.Provider(provider)] = model
	}
	if a.secrets == nil || a.secrets.Available() != nil {
		return settings, nil
	}
	settings.KeyringAvailable = true
	for _, provider := range []chat.Provider{chat.ProviderOpenAI, chat.ProviderMistral, chat.ProviderOpenRouter} {
		if _, err := a.secrets.Get(provider); err == nil {
			settings.ProvidersWithAPIKey = append(settings.ProvidersWithAPIKey, provider)
		} else if !errors.Is(err, chat.ErrAPIKeyNotFound) {
			settings.KeyringAvailable = false
			settings.ProvidersWithAPIKey = []chat.Provider{}
			break
		}
	}
	return settings, nil
}

func (a *App) UpdateChatPreferences(provider chat.Provider, model string) error {
	model = strings.TrimSpace(model)
	if err := chat.ValidatePreferences(provider, model); err != nil {
		return err
	}
	a.configMu.Lock()
	defer a.configMu.Unlock()
	next := a.config
	next.ChatProvider = string(provider)
	models := make(map[string]string, len(a.config.ChatModels)+1)
	for knownProvider, knownModel := range a.config.ChatModels {
		models[knownProvider] = knownModel
	}
	models[string(provider)] = model
	next.ChatModels = models
	if err := a.store.Save(next); err != nil {
		return err
	}
	a.config = next
	return nil
}

func (a *App) StoreChatAPIKey(provider chat.Provider, apiKey string) error {
	apiKey = strings.TrimSpace(apiKey)
	if err := chat.ValidateAPIKey(provider, apiKey); err != nil {
		return err
	}
	if a.secrets == nil {
		return chat.ErrKeyringUnavailable
	}
	return a.secrets.Set(provider, apiKey)
}

func (a *App) DeleteChatAPIKey(provider chat.Provider) error {
	if !provider.IsRemote() {
		return chat.ErrInvalidRequest
	}
	if a.secrets == nil {
		return chat.ErrKeyringUnavailable
	}
	return a.secrets.Delete(provider)
}

// ResetChatConversation oublie immédiatement l'historique anonymisé et les
// correspondances de pseudonymes gardées en mémoire pour cette conversation.
func (a *App) ResetChatConversation(conversationID string) error {
	s, release, err := a.acquireSession()
	if err != nil {
		return err
	}
	s.chatMu.Lock()
	service := s.chat
	s.chatMu.Unlock()
	release()
	if service == nil {
		return nil
	}
	return service.Reset(conversationID)
}
