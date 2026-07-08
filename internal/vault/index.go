package vault

import (
	"errors"
	"time"

	"github.com/votre-compte/notevault/internal/domain"
)

// SearchOpts paramètre une recherche full-text.
type SearchOpts struct {
	Limit int
}

// ListFilter restreint les List.
type ListFilter struct {
	Folder string
	Tag    string
	Limit  int
}

// TagCount est une étiquette avec son nombre d'occurrences.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// Index est l'interface de stockage secondaire (cache requêtable).
// Toute mutation d'une note via le Service doit être propagée à l'index.
type Index interface {
	Upsert(note domain.Note) error
	Delete(relativePath string) error
	Get(relativePath string) (domain.Note, error)
	List(filter ListFilter) ([]domain.NoteSummary, error)
	Search(query string, opts SearchOpts) ([]domain.NoteSummary, error)
	ListTags() ([]TagCount, error)
	Close() error
}

// ErrNotFound est renvoyé par Get quand la note n'existe pas dans l'index.
var ErrNotFound = errors.New("note introuvable dans l'index")

// nowUTC exporté pour les tests.
var nowUTC = func() time.Time { return time.Now().UTC() }
