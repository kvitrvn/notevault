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

func (a *App) SearchNotes(query string, limit int) ([]domain.NoteSummary, error) {
	if limit <= 0 {
		limit = 200
	}
	return a.vault.Search(query, limit)
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
