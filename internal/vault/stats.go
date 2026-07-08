package vault

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Stats agrège des indicateurs d'activité locale (30 derniers jours).
// TotalNotes et TotalWords sont absolus, les séries CreatedByDay et
// ModifiedByDay sont indexées du plus ancien au plus récent (jour 0 = N-29).
type Stats struct {
	TotalNotes    int        `json:"totalNotes"`
	TotalAssets   int64      `json:"totalAssetsBytes"`
	TotalWords    int        `json:"totalWords"`
	CreatedByDay  []DayCount `json:"createdByDay"`
	ModifiedByDay []DayCount `json:"modifiedByDay"`
	TopTags       []TagCount `json:"topTags"`
	WindowDays    int        `json:"windowDays"`
	ComputedAt    time.Time  `json:"computedAt"`
}

// DayCount associe un jour (UTC) à un nombre d'événements.
type DayCount struct {
	Day   string `json:"day"`   // YYYY-MM-DD
	Count int    `json:"count"`
}

// StatsService fenêtre d'observation (jours glissants). 30 par défaut.
const statsWindowDays = 30

// Stats calcule les indicateurs d'activité locale. Les compteurs par jour
// sont calculés en SQL (GROUP BY date(updated_at,...)). Les totaux de mots
// et la taille cumulée des assets sont lus depuis le disque (coût linéaire
// en nombre de notes, exécuté ponctuellement).
func (s *Service) Stats() (Stats, error) {
	stats := Stats{
		WindowDays: statsWindowDays,
		ComputedAt: nowUTC(),
	}
	if s.index == nil {
		return stats, fmt.Errorf("index non disponible")
	}
	buckets, err := s.index.StatsBuckets(statsWindowDays)
	if err != nil {
		return stats, fmt.Errorf("lire les compteurs : %w", err)
	}
	stats.TotalNotes = buckets.Notes
	stats.TotalWords = buckets.Words
	stats.CreatedByDay = buckets.Created
	stats.ModifiedByDay = buckets.Modified
	stats.TopTags = buckets.TopTags
	stats.TotalAssets = computeAssetsSize(s.root)
	return stats, nil
}

func computeAssetsSize(root string) int64 {
	assetsRoot := filepath.Join(root, "assets")
	var total int64
	_ = filepath.WalkDir(assetsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}

// countWords compte les mots d'un contenu Markdown. On retire les blocs
// de code et le frontmatter pour ne pas gonfler artificiellement le total.
func countWords(content string) int {
	if content == "" {
		return 0
	}
	// Retire le frontmatter YAML s'il existe.
	if strings.HasPrefix(content, "---\n") {
		if end := strings.Index(content[4:], "\n---\n"); end >= 0 {
			content = content[end+9:]
		}
	}
	// Retire les blocs de code ```...```.
	var sb strings.Builder
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		sb.WriteString(line)
		sb.WriteByte(' ')
	}
	cleaned := sb.String()
	// Supprime la syntaxe Markdown bruyante.
	for _, r := range []string{"#", "*", "_", "~", "`", ">", "[", "]", "(", ")", "!", "|", "\n", "\t"} {
		cleaned = strings.ReplaceAll(cleaned, r, " ")
	}
	fields := strings.Fields(cleaned)
	return len(fields)
}

// IsZero indique si les stats sont vides (aucune note ni asset).
func (s Stats) IsZero() bool {
	return s.TotalNotes == 0 && s.TotalAssets == 0 && s.TotalWords == 0
}

// SortTags retourne les TopTags triés (alpha) — utile pour les tests.
func (s Stats) SortTags() []TagCount {
	out := append([]TagCount(nil), s.TopTags...)
	sort.Slice(out, func(i, j int) bool { return out[i].Tag < out[j].Tag })
	return out
}
