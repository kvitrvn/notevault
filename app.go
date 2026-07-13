package main

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	"github.com/kvitrvn/notevault/internal/config"
	"github.com/kvitrvn/notevault/internal/domain"
	"github.com/kvitrvn/notevault/internal/vault"
)

// App est la façade explicitement exposée au frontend Wails.
// Garder cette couche mince : la logique métier doit rester dans internal/.
type App struct {
	ctx       context.Context
	vault     *vault.Service
	assetSrv  *vault.AssetServer
	assetPort int
}

func NewApp() (*App, error) {
	service, err := vault.NewDefaultService()
	if err != nil {
		return nil, fmt.Errorf("initialiser le coffre : %w", err)
	}
	// Un coffre chiffré reste vierge en mémoire jusqu'au déverrouillage.
	if status := service.VaultStatus(); status.State != vault.VaultLocked {
		if err := service.IndexNow(service.BootstrapContext(), nil); err != nil {
			return nil, fmt.Errorf("indexation initiale : %w", err)
		}
	}

	assetSrv := vault.NewAssetServer(service.Root())
	port, err := assetSrv.Start()
	if err != nil {
		_ = service.Close()
		return nil, fmt.Errorf("démarrer asset server : %w", err)
	}

	return &App{vault: service, assetSrv: assetSrv, assetPort: port}, nil
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) Shutdown(ctx context.Context) {
	if a.assetSrv != nil {
		_ = a.assetSrv.Stop()
	}
	_ = a.vault.Close()
}

func (a *App) VaultPath() string {
	return a.vault.Root()
}

func (a *App) VaultStatus() vault.VaultStatusInfo {
	return a.vault.VaultStatus()
}

func (a *App) EnableEncryption(passphrase string) error {
	return a.vault.EnableEncryption(passphrase)
}

func (a *App) UnlockVault(passphrase string) error {
	return a.vault.UnlockVault(passphrase)
}

func (a *App) ChangePassphrase(current, replacement string) error {
	return a.vault.ChangePassphrase(current, replacement)
}

func (a *App) DisableEncryption(passphrase string) error {
	return a.vault.DisableEncryption(passphrase)
}

func (a *App) ListNotes() ([]domain.NoteSummary, error) {
	return a.vault.ListNotes()
}

// ListNotesFiltered applique une requête structurée (filtre sidebar).
func (a *App) ListNotesFiltered(filter vault.FilterQuery, limit int) ([]domain.NoteSummary, error) {
	if limit <= 0 {
		limit = 1000
	}
	return a.vault.ListNotesFiltered(filter, limit)
}

// ListPinned retourne les notes épinglées.
func (a *App) ListPinned() ([]domain.NoteSummary, error) {
	return a.vault.ListPinned()
}

// ListFolders retourne les dossiers connus du coffre.
func (a *App) ListFolders() ([]vault.FolderInfo, error) {
	return a.vault.ListFolders()
}

// PinNote épingle ou désépingle une note.
func (a *App) PinNote(relativePath string, pinned bool) error {
	return a.vault.Pin(relativePath, pinned)
}

// IsNotePinned indique si une note est épinglée.
func (a *App) IsNotePinned(relativePath string) (bool, error) {
	return a.vault.IsPinned(relativePath)
}

// ListTags retourne les tags connus.
func (a *App) ListTags() ([]vault.TagCount, error) {
	return a.vault.ListTags()
}

func (a *App) SearchNotes(query string, limit int) ([]domain.NoteSummary, error) {
	if limit <= 0 {
		limit = 200
	}
	return a.vault.Search(query, limit)
}

// OpenDailyNote ouvre (ou crée) la note du jour.
func (a *App) OpenDailyNote() (domain.Note, error) {
	return a.vault.OpenDailyNote()
}

// EnsureDailyNote crée la note du jour si AutoDailyNote=true.
// Retourne le chemin relatif ou "" si la fonction est désactivée.
func (a *App) EnsureDailyNote() (string, error) {
	return a.vault.EnsureDailyNote()
}

// ListTemplates retourne la liste des templates disponibles.
func (a *App) ListTemplates() []vault.Template {
	return a.vault.ListTemplates()
}

// CreateNoteFromTemplate crée une note avec un template nommé (id de Template).
// Si templateID est vide ou "blank", la note est vide.
func (a *App) CreateNoteFromTemplate(title, templateID string) (domain.Note, error) {
	return a.vault.CreateNote(title, templateID)
}

// MoveNote déplace une note vers un nouveau chemin.
func (a *App) MoveNote(oldPath, newPath string) (domain.Note, error) {
	return a.vault.MoveNote(oldPath, newPath)
}

// DuplicateNote crée une copie d'une note.
func (a *App) DuplicateNote(relativePath string) (domain.Note, error) {
	return a.vault.DuplicateNote(relativePath)
}

// OpenInExplorer ouvre le fichier (ou son dossier) dans le gestionnaire natif.
func (a *App) OpenInExplorer(relativePath string, reveal bool) error {
	return a.vault.OpenInExplorer(relativePath, reveal)
}

// RenameTitle met à jour uniquement le titre d'une note.
func (a *App) RenameTitle(relativePath, newTitle string) (domain.Note, error) {
	return a.vault.RenameTitle(relativePath, newTitle)
}

// GetBacklinks retourne les notes qui mentionnent le titre donné.
// excludePath permet d'ignorer la note courante des résultats.
func (a *App) GetBacklinks(title, excludePath string, limit int) ([]domain.NoteSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	return a.vault.GetBacklinks(title, excludePath, limit)
}

// SaveAsset enregistre un binaire (image, etc.) dans le coffre.
// `filename` est utilisé pour deviner l'extension.
func (a *App) SaveAsset(data []byte, filename string) (string, error) {
	return a.vault.SaveAsset(data, filename)
}

// ImportAssetFromFilePath copie un fichier existant (par exemple dropé
// depuis un explorateur de fichiers) dans le coffre. Voir
// vault.Service.ImportAssetFromFilePath.
func (a *App) ImportAssetFromFilePath(absolutePath string) (string, error) {
	return a.vault.ImportAssetFromFilePath(absolutePath)
}

// ListHistory retourne les versions d'une note.
func (a *App) ListHistory(relativePath string) ([]vault.HistoryEntry, error) {
	return a.vault.ListHistory(relativePath)
}

// ReadHistoryVersion retourne le contenu brut d'une version d'historique.
func (a *App) ReadHistoryVersion(relativePath, versionID string) (string, error) {
	return a.vault.ReadHistoryVersion(relativePath, versionID)
}

// RestoreFromHistory restaure une version comme version courante.
func (a *App) RestoreFromHistory(relativePath, versionID string) (domain.Note, error) {
	return a.vault.RestoreFromHistory(relativePath, versionID)
}

// DiffHistory retourne le diff unifié entre deux versions.
func (a *App) DiffHistory(relativePath, aID, bID string) (string, error) {
	return a.vault.DiffHistory(relativePath, aID, bID)
}

// OpenAsset retourne le chemin absolu d'un asset (utilisable par le frontend
// pour l'afficher via file:// ou équivalent Wails).
func (a *App) OpenAsset(relativePath string) (string, error) {
	return a.vault.ResolveAsset(relativePath)
}

// AssetURL retourne l'URL HTTP locale à utiliser dans `<img src=...>` pour
// afficher un asset. Le serveur HTTP interne est démarré dans NewApp et
// confine les requêtes à <vault>/assets/ avec une whitelist d'extensions.
func (a *App) AssetURL(relativePath string) (string, error) {
	abs, err := a.vault.ResolveAsset(relativePath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(a.vault.Root(), abs)
	if err != nil {
		return "", fmt.Errorf("construire l'URL de l'asset : %w", err)
	}
	assetURL := url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("127.0.0.1:%d", a.assetPort),
		Path:   "/files/" + filepath.ToSlash(rel),
	}
	return assetURL.String(), nil
}

func (a *App) OpenNote(relativePath string) (domain.Note, error) {
	return a.vault.OpenNote(relativePath)
}

func (a *App) CreateNote(title string, templateKey string) (domain.Note, error) {
	return a.vault.CreateNote(title, templateKey)
}

func (a *App) SaveNote(note domain.Note) (domain.Note, error) {
	return a.vault.SaveNote(note)
}

func (a *App) DeleteNote(relativePath string) error {
	return a.vault.DeleteNote(relativePath)
}

// ListTrash retourne les notes actuellement en corbeille.
func (a *App) ListTrash() ([]vault.TrashEntry, error) {
	return a.vault.ListTrash()
}

// RestoreFromTrash remet en place une note supprimée.
func (a *App) RestoreFromTrash(id string) (domain.Note, error) {
	return a.vault.RestoreFromTrash(id)
}

// EmptyTrash vide la corbeille.
func (a *App) EmptyTrash() error {
	return a.vault.EmptyTrash()
}

// GetConfig retourne la configuration persistée.
func (a *App) GetConfig() (config.Config, error) {
	return a.vault.GetConfig()
}

// UpdateConfig enregistre la configuration.
func (a *App) UpdateConfig(cfg config.Config) error {
	return a.vault.UpdateConfig(cfg)
}

// --- Phase 5 : thèmes, export, stats, recovery ---------------------------

// ListThemes retourne les thèmes disponibles (built-in + utilisateur).
func (a *App) ListThemes() []vault.Theme {
	return a.vault.ListThemes()
}

// Theme retourne un thème par ID.
func (a *App) Theme(id string) (vault.Theme, error) {
	return a.vault.Theme(id)
}

// ExportNotes écrit les notes fournies (+ assets) dans un fichier zip.
func (a *App) ExportNotes(paths []string, destZip string) error {
	return a.vault.ExportNotes(paths, destZip)
}

// Stats calcule les indicateurs d'activité locale.
func (a *App) Stats() (vault.Stats, error) {
	return a.vault.Stats()
}

// SnapshotForStartup renvoie l'état combiné onboarding + recovery.
func (a *App) SnapshotForStartup() (vault.RecoverySnapshot, error) {
	return a.vault.SnapshotForStartup()
}

// MarkOnboardingCompleted marque l'onboarding comme terminé.
func (a *App) MarkOnboardingCompleted(onboarding *vault.Onboarding) error {
	return a.vault.MarkOnboardingCompleted(onboarding)
}

// SetDirtyBuffer enregistre un buffer en cours d'édition (recovery).
func (a *App) SetDirtyBuffer(notePath, buffer string, diskMTime time.Time) error {
	return a.vault.SetDirtyBuffer(notePath, buffer, diskMTime)
}

// ClearDirtyBuffer efface le buffer de recovery.
func (a *App) ClearDirtyBuffer() error {
	return a.vault.ClearDirtyBuffer()
}
