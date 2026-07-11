//go:build bindings

package main

// La génération Wails exécute un binaire compilé avec le tag "bindings".
// Une valeur vide suffit à l'introspection et évite d'ouvrir le coffre, SQLite
// ou un port HTTP pendant une simple génération de code.
func newApplication() (*App, error) {
	return &App{}, nil
}
