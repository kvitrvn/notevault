package vault

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// FilterQuery est la requête de l'utilisateur telle qu'elle a été
// comprise par le parser. Tous les champs sont des AND.
type FilterQuery struct {
	Query       string    // texte brut à envoyer à FTS5
	Tags        []string  // tous requis
	ExcludeTags []string  // aucun ne doit être présent
	Path        string    // préfixe de chemin (sans le notes/)
	UpdatedFrom time.Time // updated_at >= UpdatedFrom
	UpdatedTo   time.Time // updated_at <  UpdatedTo
}

// String retourne la requête d'origine pour affichage.
func (f FilterQuery) String() string {
	if f.IsEmpty() {
		return ""
	}
	return strings.TrimSpace(f.Query)
}

// IsEmpty indique qu'aucun filtre n'est actif.
func (f FilterQuery) IsEmpty() bool {
	return f.Query == "" &&
		len(f.Tags) == 0 &&
		len(f.ExcludeTags) == 0 &&
		f.Path == "" &&
		f.UpdatedFrom.IsZero() &&
		f.UpdatedTo.IsZero()
}

// ToListFilter convertit vers le ListFilter attendu par l'index.
func (f FilterQuery) ToListFilter(limit int) ListFilter {
	return ListFilter{
		Query:       f.Query,
		Tags:        f.Tags,
		ExcludeTags: f.ExcludeTags,
		Folder:      f.Path,
		UpdatedFrom: f.UpdatedFrom,
		UpdatedTo:   f.UpdatedTo,
		Limit:       limit,
	}
}

// ActiveChips décrit les filtres structurés actifs pour affichage
// dans la sidebar (un chip par filtre, cliquable pour le retirer).
type ActiveChip struct {
	Kind string // "tag" | "exclude" | "path" | "updatedFrom" | "updatedTo"
	Text string
}

// Chips retourne les chips ordonnés pour affichage.
func (f FilterQuery) Chips() []ActiveChip {
	out := make([]ActiveChip, 0)
	for _, t := range f.Tags {
		out = append(out, ActiveChip{Kind: "tag", Text: t})
	}
	for _, t := range f.ExcludeTags {
		out = append(out, ActiveChip{Kind: "exclude", Text: "-" + t})
	}
	if f.Path != "" {
		out = append(out, ActiveChip{Kind: "path", Text: f.Path})
	}
	if !f.UpdatedFrom.IsZero() {
		out = append(out, ActiveChip{Kind: "updatedFrom", Text: "≥ " + formatDate(f.UpdatedFrom)})
	}
	if !f.UpdatedTo.IsZero() {
		out = append(out, ActiveChip{Kind: "updatedTo", Text: "< " + formatDate(f.UpdatedTo)})
	}
	return out
}

// ParseFilter analyse une requête utilisateur et retourne un FilterQuery.
//
// Grammaire (informelle) :
//
//	query     := term (WS term)*
//	term      := '-' ? ( filter | WORD | "..." )
//	filter    := KEY ':' VALUE
//	KEY       := 'tag' | 'path' | 'updated'
//	VALUE     := jusqu'au prochain WS ou filtre suivant
//	"..."     := phrase conservée telle quelle (FTS5 phrase)
func ParseFilter(input string) (FilterQuery, error) {
	fq := FilterQuery{}
	tokens, err := tokenize(input)
	if err != nil {
		return fq, err
	}
	plain := make([]string, 0, len(tokens))
	for _, t := range tokens {
		switch t.kind {
		case tokenFilter:
			key, value, ok := splitFilter(t.value)
			if !ok {
				return fq, fmt.Errorf("filtre mal formé : %q", t.value)
			}
			if err := applyFilter(&fq, key, value); err != nil {
				return fq, err
			}
		case tokenWord:
			plain = append(plain, t.value)
		case tokenPhrase:
			plain = append(plain, `"`+t.value+`"`)
		}
	}
	fq.Query = strings.Join(plain, " ")
	return fq, nil
}

// RemoveChip retourne une nouvelle FilterQuery sans le chip ciblé.
// kind et text identifient un chip de f.Chips() (premier match).
func RemoveChip(f FilterQuery, kind, text string) FilterQuery {
	out := f
	switch kind {
	case "tag":
		out.Tags = removeFromSlice(out.Tags, normalizeTag(text))
	case "exclude":
		out.ExcludeTags = removeFromSlice(out.ExcludeTags, normalizeTag(strings.TrimPrefix(text, "-")))
	case "path":
		if out.Path == text {
			out.Path = ""
		}
	case "updatedFrom":
		out.UpdatedFrom = time.Time{}
	case "updatedTo":
		out.UpdatedTo = time.Time{}
	}
	return out
}

type tokenKind int

const (
	tokenWord tokenKind = iota
	tokenPhrase
	tokenFilter
)

type token struct {
	kind  tokenKind
	value string
}

// tokenize découpe input en tokens en respectant les guillemets et les filtres
// préfixés (clé:).
func tokenize(input string) ([]token, error) {
	out := make([]token, 0)
	var buf strings.Builder
	flushWord := func() {
		if buf.Len() == 0 {
			return
		}
		v := buf.String()
		buf.Reset()
		if idx := splitFilterKey(v); isFilterKey(idx) {
			out = append(out, token{kind: tokenFilter, value: v})
		} else {
			out = append(out, token{kind: tokenWord, value: v})
		}
	}
	i := 0
	for i < len(input) {
		c := input[i]
		switch c {
		case ' ', '\t', '\n':
			flushWord()
			i++
		case '"':
			// Phrase entre guillemets.
			i++
			start := i
			for i < len(input) && input[i] != '"' {
				i++
			}
			if i >= len(input) {
				return nil, fmt.Errorf("guillemet ouvrant non fermé")
			}
			out = append(out, token{kind: tokenPhrase, value: input[start:i]})
			i++ // consomme le guillemet fermant
		default:
			// Lecture jusqu'au prochain espace ou guillemet.
			start := i
			for i < len(input) && input[i] != ' ' && input[i] != '\t' && input[i] != '\n' && input[i] != '"' {
				i++
			}
			buf.WriteString(input[start:i])
			flushWord()
		}
	}
	flushWord()
	return out, nil
}

// splitFilterKey retourne l'index du premier ':' si la chaîne ressemble à un filtre.
func splitFilterKey(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return i
		}
	}
	return -1
}

func isFilterKey(idx int) bool { return idx > 0 }

func splitFilter(s string) (key, value string, ok bool) {
	idx := splitFilterKey(s)
	if idx <= 0 || idx == len(s)-1 {
		return "", "", false
	}
	return s[:idx], s[idx+1:], true
}

func applyFilter(fq *FilterQuery, key, value string) error {
	switch strings.ToLower(key) {
	case "tag":
		fq.Tags = append(fq.Tags, normalizeTag(value))
	case "-tag":
		fq.ExcludeTags = append(fq.ExcludeTags, normalizeTag(value))
	case "path":
		// Accepte "projects" ou "projects/" ou "projects/*".
		v := strings.TrimSpace(value)
		v = strings.TrimSuffix(v, "/*")
		v = strings.TrimSuffix(v, "/")
		if v != "" {
			fq.Path = v
		}
	case "updated":
		from, to, err := parseDateValue(value)
		if err != nil {
			return err
		}
		if !from.IsZero() {
			fq.UpdatedFrom = from
		}
		if !to.IsZero() {
			fq.UpdatedTo = to
		}
	default:
		return fmt.Errorf("filtre inconnu : %q", key)
	}
	return nil
}

// parseDateValue gère plusieurs formes :
//   - "today"           → [aujourd'hui 00:00, demain 00:00)
//   - "yesterday"       → [hier 00:00, aujourd'hui 00:00)
//   - "2026-01-01"      → [date 00:00, date+1 00:00)
//   - ">2026-01-01"     → [date+1 00:00, zero)
//   - "<2026-01-01"     → [zero, date 00:00)
//   - ">=2026-01-01"    → [date 00:00, zero)
//   - "<=2026-01-01"    → [zero, date+1 00:00)
func parseDateValue(s string) (time.Time, time.Time, error) {
	s = strings.TrimSpace(s)
	now := nowUTC()
	loc := now.Location()
	switch strings.ToLower(s) {
	case "today":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).UTC()
		return start, start.AddDate(0, 0, 1), nil
	case "yesterday":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).UTC().AddDate(0, 0, -1)
		return start, start.AddDate(0, 0, 1), nil
	case "thisweek":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).UTC().AddDate(0, 0, -int(now.Weekday()))
		if now.Weekday() == 0 {
			start = start.AddDate(0, 0, -7)
		}
		return start, start.AddDate(0, 0, 7), nil
	}
	if len(s) == 0 {
		return time.Time{}, time.Time{}, fmt.Errorf("valeur de date vide")
	}
	var cmp byte = '='
	rest := s
	if len(s) > 1 {
		switch {
		case s[0] == '>' && s[1] == '=':
			cmp = 'G'
			rest = s[2:]
		case s[0] == '<' && s[1] == '=':
			cmp = 'l'
			rest = s[2:]
		case s[0] == '>':
			cmp = 'g'
			rest = s[1:]
		case s[0] == '<':
			cmp = 'L'
			rest = s[1:]
		}
	}
	d, err := time.ParseInLocation("2006-01-02", rest, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("date invalide : %q", s)
	}
	d = d.UTC()
	switch cmp {
	case 'G', 'g':
		return d, time.Time{}, nil
	case 'l':
		// <= date : UpdatedTo = date + 1 jour (exclusif)
		return time.Time{}, d.AddDate(0, 0, 1), nil
	case 'L':
		// < date : UpdatedTo = date (exclusif)
		return time.Time{}, d, nil
	default:
		return d, d.AddDate(0, 0, 1), nil
	}
}

func normalizeTag(s string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "#"))
}

func removeFromSlice(in []string, target string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == target {
			continue
		}
		out = append(out, v)
	}
	return out
}

func formatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// itoa évite strconv dans un hot-path (utilisé seulement dans les parse dates).
func itoa(n int) string { return strconv.Itoa(n) }
