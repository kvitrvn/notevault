package main

import (
	"context"
	"fmt"

	"github.com/votre-compte/notevault/internal/config"
	"github.com/votre-compte/notevault/internal/domain"
	"github.com/votre-compte/notevault/internal/vault"
)

// App est la façade explicitement exposée au frontend Wails.
// Garder cette couche mince : la logique métier doit rester dans internal/.
type App struct {
	ctx   context.Context
	vault *vault.Service
}

func NewApp() (*App, error) {
	service, err := vault.NewDefaultService()
	if err != nil {
		return nil, fmt.Errorf("initialiser le coffre : %w", err)
	}
	// Indexation initiale synchrone : bloque Startup tant que l'index
	// n'est pas prêt. À 10 000 notes cible, l'opération doit rester < 2 s.
	if err := service.IndexNow(service.BootstrapContext(), nil); err != nil {
		return nil, fmt.Errorf("indexation initiale : %w", err)
	}
	return &App{vault: service}, nil
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) Shutdown(ctx context.Context) {
	_ = a.vault.Close()
}

func (a *App) VaultPath() string {
	return a.vault.Root()
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
