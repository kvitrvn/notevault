package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Template représente un modèle de note, utilisable par CreateNote.
// ID est un identifiant court (slug du nom de fichier ou clé de built-in).
// Name est le nom affiché dans le picker.
// Body est le contenu Markdown (sans frontmatter).
// Builtin true pour les templates en dur dans markdown.go.
type Template struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Body    string `json:"body"`
	Builtin bool   `json:"builtin"`
}

// TemplateLoader lit les templates utilisateur depuis <root>/templates/*.md.
// Le format est : frontmatter optionnel `name: "..."`, puis le contenu Markdown.
type TemplateLoader struct {
	root string
}

// NewTemplateLoader crée un loader pour le coffre donné.
func NewTemplateLoader(root string) *TemplateLoader {
	return &TemplateLoader{root: root}
}

// List retourne tous les templates disponibles : d'abord les built-in,
// puis les templates utilisateur triés par nom. Si un template utilisateur
// a le même ID qu'un built-in, le user écrase le built-in (utile pour
// personnaliser un modèle par défaut).
func (l *TemplateLoader) List() []Template {
	out := make([]Template, 0, 8)
	seen := make(map[string]int)
	for _, t := range builtinTemplates() {
		seen[t.ID] = len(out)
		out = append(out, t)
	}
	tplDir := filepath.Join(l.root, "templates")
	entries, err := os.ReadDir(tplDir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.EqualFold(filepath.Ext(name), ".md") {
			continue
		}
		t, err := parseTemplateFile(filepath.Join(tplDir, name))
		if err != nil {
			continue
		}
		if existing, ok := seen[t.ID]; ok {
			out[existing] = t
			continue
		}
		seen[t.ID] = len(out)
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get retourne le template par ID. Renvoie une erreur si introuvable.
func (l *TemplateLoader) Get(id string) (Template, error) {
	for _, t := range l.List() {
		if t.ID == id {
			return t, nil
		}
	}
	return Template{}, fmt.Errorf("template introuvable : %q", id)
}

func parseTemplateFile(path string) (Template, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Template{}, err
	}
	body := string(raw)
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	id := slug(name)
	body = strings.ReplaceAll(body, "\r\n", "\n")
	if strings.HasPrefix(body, "---\n") {
		if end := strings.Index(body[4:], "\n---\n"); end >= 0 {
			front := body[4 : end+4]
			body = strings.TrimPrefix(body[end+9:], "\n")
			for _, line := range strings.Split(front, "\n") {
				k, v, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				if strings.TrimSpace(k) == "name" {
					if trimmed := strings.Trim(strings.TrimSpace(v), "\"'"); trimmed != "" {
						name = trimmed
					}
				}
			}
		}
	}
	if id == "" {
		id = "user-" + name
	}
	return Template{ID: id, Name: name, Body: body, Builtin: false}, nil
}

// builtinTemplates retourne les modèles définis en dur dans markdown.go.
func builtinTemplates() []Template {
	return []Template{
		{ID: "blank", Name: "Note vide", Body: "", Builtin: true},
		{ID: "meeting", Name: "Compte-rendu de réunion", Body: template("meeting"), Builtin: true},
		{ID: "daily", Name: "Journal quotidien", Body: template("daily"), Builtin: true},
	}
}

// errEmptyTemplateDir est supprimé (inutile).

// ensureTemplateDir crée le dossier templates s'il n'existe pas.
// Appelé au bootstrap pour faciliter l'ajout de templates utilisateur.
func ensureTemplateDir(root string) error {
	return os.MkdirAll(filepath.Join(root, "templates"), 0o755)
}
