package main

import (
	"context"
	"fmt"

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
	return &App{vault: service}, nil
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) VaultPath() string {
	return a.vault.Root()
}

func (a *App) ListNotes() ([]domain.NoteSummary, error) {
	return a.vault.ListNotes()
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
