package vault

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kvitrvn/notevault/internal/domain"
	"gopkg.in/yaml.v3"
)

// frontmatterSchema regroupe les champs connus de NoteVault. Tout autre
// champ YAML rencontré est reporté dans Note.ExtraFrontmatter et
// préservé tel quel à la sauvegarde. C'est le contrat "vault = source
// de vérité" : seules nos 4 propriétés sont normalisées, le reste est
// traité comme opaque pour NoteVault.
type frontmatterSchema struct {
	Title   any            `yaml:"title,omitempty"`
	Created string         `yaml:"created,omitempty"`
	Updated string         `yaml:"updated,omitempty"`
	Tags    yaml.Node      `yaml:"tags,omitempty"`
	Extra   map[string]any `yaml:",inline"`
}

// reservedFrontmatterKeys est l'ensemble des clés que NoteVault
// contrôle. Utilisé pour déplacer ces clés hors du map "extras"
// lorsqu'on reconstruit le frontmatter.
var reservedFrontmatterKeys = map[string]bool{
	"title":   true,
	"created": true,
	"updated": true,
	"tags":    true,
}

func serialize(note domain.Note) string {
	fm := frontmatterSchema{
		Title:   note.Title,
		Created: note.CreatedAt.UTC().Format(time.RFC3339),
		Updated: note.UpdatedAt.UTC().Format(time.RFC3339),
		Extra:   make(map[string]any, len(note.ExtraFrontmatter)),
	}
	if note.Tags != nil {
		fm.Tags = yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, tag := range note.Tags {
			fm.Tags.Content = append(fm.Tags.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: tag})
		}
	}
	for k, v := range note.ExtraFrontmatter {
		if reservedFrontmatterKeys[strings.ToLower(k)] {
			continue
		}
		fm.Extra[k] = v
	}
	// Tri alphabétique des clés utilisateur pour un diff stable.
	if len(fm.Extra) > 1 {
		keys := make([]string, 0, len(fm.Extra))
		for k := range fm.Extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		sorted := make(map[string]any, len(fm.Extra))
		for _, k := range keys {
			sorted[k] = fm.Extra[k]
		}
		fm.Extra = sorted
	}
	body := strings.TrimSpace(note.Content)
	var buf bytes.Buffer
	buf.WriteString("---\n")
	if err := yamlEncoder(&buf).Encode(fm); err != nil {
		// Fallback : on n'écrit pas de frontmatter plutôt que de
		// corrompre la note. Le contenu corporel est conservé tel quel.
		buf.Reset()
		buf.WriteString("---\n")
	} else {
		// yaml.Encoder ajoute un "\n" final ; on le laisse, le closer
		// "---" qui suit crée la séparation correcte.
	}
	// Si le YAML produit ne se termine pas par un newline (cas vide),
	// on garantit la fermeture.
	out := buf.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return fmt.Sprintf("%s---\n\n%s\n", out, body)
}

// yamlEncoder retourne un encoder YAML déterministe (indent 2).
// Le format reste compatible avec un parse standard.
func yamlEncoder(buf *bytes.Buffer) *yaml.Encoder {
	enc := yaml.NewEncoder(buf)
	enc.SetIndent(2)
	return enc
}

// parse extrait les champs connus et conserve les clés inconnues
// dans Note.ExtraFrontmatter. Le contenu corporel reste inchangé.
func parse(raw string) domain.Note {
	note := domain.Note{Tags: []string{}}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(raw, "---\n") {
		note.Content = raw
		return note
	}
	end := strings.Index(raw[4:], "\n---\n")
	if end < 0 {
		// Frontmatter ouvrant sans fermeture : on traite le fichier
		// entier comme corps. Le save suivant reconstruira proprement.
		note.Content = raw
		return note
	}
	front := raw[4 : end+4]
	body := raw[end+9:]
	body = strings.TrimPrefix(body, "\n")
	note.Content = body

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(front), &parsed); err != nil {
		// YAML invalide : on tente une récupération minimale via le
		// parser ligne par ligne historique, puis on retourne.
		return fallbackParse(front, note)
	}
	if v, ok := parsed["title"]; ok {
		if s, ok := v.(string); ok {
			note.Title = s
		}
	}
	if v, ok := parsed["created"]; ok {
		if s, ok := v.(string); ok {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				note.CreatedAt = t
			}
		}
	}
	if v, ok := parsed["updated"]; ok {
		if s, ok := v.(string); ok {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				note.UpdatedAt = t
			}
		}
	}
	if v, ok := parsed["tags"]; ok {
		note.Tags = coerceTags(v)
	}
	extras := make(map[string]any)
	for k, v := range parsed {
		if reservedFrontmatterKeys[strings.ToLower(k)] {
			continue
		}
		extras[k] = v
	}
	if len(extras) > 0 {
		note.ExtraFrontmatter = extras
	}
	return note
}

// fallbackParse tente une lecture ligne par ligne quand le YAML
// n'est pas parsable. Préserve au mieux les clés inconnues pour ne
// pas régresser par rapport à l'ancien comportement.
func fallbackParse(front string, note domain.Note) domain.Note {
	extras := make(map[string]any)
	for _, line := range strings.Split(front, "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if reservedFrontmatterKeys[strings.ToLower(key)] {
			continue
		}
		if key == "" {
			continue
		}
		extras[key] = value
	}
	if len(extras) > 0 {
		note.ExtraFrontmatter = extras
	}
	return note
}

func coerceTags(v any) []string {
	switch x := v.(type) {
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	case string:
		// YAML peut renvoyer "a, b" inline si l'auteur l'a écrit ainsi.
		out := make([]string, 0)
		for _, part := range strings.Split(x, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
		return out
	}
	return nil
}

func escapeYAMLScalar(value string) string {
	// Conservé pour les chemins legacy (non utilisé désormais, mais on
	// garde l'API pour les tests tiers).
	return strings.ReplaceAll(strings.TrimSpace(value), "\n", " ")
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r >= 'à' && r <= 'ÿ':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "note"
	}
	return result
}

func template(key string) string {
	switch key {
	case "meeting":
		return "# Participants\n\n- \n\n# Ordre du jour\n\n- \n\n# Décisions\n\n- \n\n# Actions\n\n- [ ] \n"
	case "daily":
		return "# Intention\n\n\n# Journal\n\n\n# À retenir\n\n\n# Demain\n\n- [ ] \n"
	default:
		return ""
	}
}
