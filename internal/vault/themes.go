package vault

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Theme est un jeu de variables CSS surchargeant les couleurs par défaut.
// Name est le libellé affiché dans le menu. Active distingue les thèmes
// built-in (intégrés en dur dans styles.css) des thèmes utilisateur
// chargés depuis le dossier themes/.
type Theme struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Vars    map[string]string `json:"vars"`
	Builtin bool              `json:"builtin"`
}

// ThemeLoader lit les thèmes utilisateur depuis <root>/.notevault/themes/*.json.
// Le format est : { "name": "Mon thème", "vars": { "--color-accent": "#..." } }.
type ThemeLoader struct {
	root string
}

// NewThemeLoader crée un loader pour le coffre donné.
func NewThemeLoader(root string) *ThemeLoader {
	return &ThemeLoader{root: root}
}

// List retourne tous les thèmes disponibles : built-in d'abord, puis les
// thèmes utilisateur triés par nom. Un thème utilisateur peut écraser un
// built-in en réutilisant le même ID (par exemple "dark").
func (l *ThemeLoader) List() []Theme {
	out := make([]Theme, 0, 4)
	seen := make(map[string]int)
	for _, t := range builtinThemes() {
		seen[t.ID] = len(out)
		out = append(out, t)
	}
	themesDir := filepath.Join(l.root, ".notevault", "themes")
	entries, err := os.ReadDir(themesDir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(e.Name()), ".json") {
			continue
		}
		t, err := parseThemeFile(filepath.Join(themesDir, e.Name()))
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
	sort.Slice(out, func(i, j int) bool {
		if out[i].Builtin != out[j].Builtin {
			return out[i].Builtin
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Get retourne un thème par ID. Renvoie une erreur si introuvable.
func (l *ThemeLoader) Get(id string) (Theme, error) {
	for _, t := range l.List() {
		if t.ID == id {
			return t, nil
		}
	}
	return Theme{}, fmt.Errorf("thème introuvable : %q", id)
}

func parseThemeFile(path string) (Theme, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Theme{}, err
	}
	var t Theme
	if err := json.Unmarshal(raw, &t); err != nil {
		return Theme{}, fmt.Errorf("JSON invalide : %w", err)
	}
	if t.ID == "" {
		t.ID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if t.Name == "" {
		t.Name = t.ID
	}
	if t.Vars == nil {
		t.Vars = map[string]string{}
	}
	for k := range t.Vars {
		if !strings.HasPrefix(k, "--") {
			delete(t.Vars, k)
		}
	}
	return t, nil
}

// builtinThemes reflète les jeux de couleurs définis en dur dans styles.css.
// Les vars sont injectées en CSS variables sur l'élément :root quand un
// thème non built-in est sélectionné.
func builtinThemes() []Theme {
	return []Theme{
		{
			ID:      "dark",
			Name:    "Sombre",
			Vars:    map[string]string{},
			Builtin: true,
		},
		{
			ID:      "light",
			Name:    "Clair",
			Vars:    map[string]string{},
			Builtin: true,
		},
	}
}

// ensureThemeDir crée le dossier themes s'il n'existe pas.
func ensureThemeDir(root string) error {
	return os.MkdirAll(filepath.Join(root, ".notevault", "themes"), 0o755)
}
