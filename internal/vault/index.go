package vault

import (
	"errors"
	"time"

	"github.com/kvitrvn/notevault/internal/domain"
)

// SearchOpts paramètre une recherche full-text.
type SearchOpts struct {
	Limit       int
	TitleWeight float64 // pondération (futur : pour le tri)
	ExcludePath string  // chemin relatif à exclure des résultats
}

// ListFilter restreint les List.
//
// Zero value valide : équivaut à "toutes les notes triées par updated_at DESC".
type ListFilter struct {
	Folder      string    // préfixe de chemin (notes/projects/)
	Tag         string    // tag unique (legacy, conservé pour compat)
	Tags        []string  // tous ces tags doivent être présents (AND)
	ExcludeTags []string  // aucun de ces tags ne doit être présent (NOT)
	UpdatedFrom time.Time // updated_at >= UpdatedFrom (si non zéro)
	UpdatedTo   time.Time // updated_at < UpdatedTo (si non zéro)
	Query       string    // expression FTS5 sur titre + contenu
	Limit       int
}

// TagCount est une étiquette avec son nombre d'occurrences.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// FolderInfo décrit un dossier du coffre pour la vue arborescente.
type FolderInfo struct {
	Path  string `json:"path"`  // chemin relatif à notes/ ("projects/web")
	Name  string `json:"name"`  // dernier segment ("web")
	Count int    `json:"count"` // nombre de notes dans ce dossier
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
	ListFolders() ([]FolderInfo, error)
	Pin(relativePath string, pinned bool) error
	ListPinned() ([]domain.NoteSummary, error)
	IsPinned(relativePath string) (bool, error)
	GetBacklinks(title string, opts SearchOpts) ([]domain.NoteSummary, error)
	StatsBuckets(windowDays int) (StatsBucketsResult, error)
	Close() error
}

// StatsBucketsResult regroupe les agrégats calculés par l'index.
// Created/Modified sont des séries ordonnées du plus ancien au plus récent,
// alignées sur la fenêtre [now-windowDays, now). Words est la somme des
// compteurs de mots calculée en mémoire.
type StatsBucketsResult struct {
	Created  []DayCount
	Modified []DayCount
	Notes    int
	Words    int
	TopTags  []TagCount
}

// ErrNotFound est renvoyé par Get quand la note n'existe pas dans l'index.
var ErrNotFound = errors.New("note introuvable dans l'index")

// ErrFolderExists est renvoyé par CreateFolder quand le dossier cible existe déjà.
var ErrFolderExists = errors.New("dossier déjà existant")

// nowUTC exporté pour les tests.
var nowUTC = func() time.Time { return time.Now().UTC() }
