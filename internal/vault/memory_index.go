package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/kvitrvn/notevault/internal/domain"
	"golang.org/x/text/unicode/norm"
)

type pinRecord struct {
	RelativePath string    `json:"relativePath"`
	PinnedAt     time.Time `json:"pinnedAt"`
}

type pinsFile struct {
	Version int         `json:"version"`
	Pins    []pinRecord `json:"pins"`
}

type rankedSummary struct {
	summary domain.NoteSummary
	score   int
}

var wikiLinkPattern = regexp.MustCompile(`\[\[([^\]\n]+?)\]\]`)

// memoryIndex is the complete process-local secondary index. Vault files are
// the source of truth; only pin order is persisted.
type memoryIndex struct {
	mu           sync.RWMutex
	notes        map[string]domain.Note
	tokens       map[string]map[string]int
	noteKeys     map[string]map[string]int
	foldedBodies map[string]string
	pins         map[string]time.Time
	pinsPath     string
}

func newMemoryIndex(root string) (Index, error) {
	i := &memoryIndex{
		notes:        make(map[string]domain.Note),
		tokens:       make(map[string]map[string]int),
		noteKeys:     make(map[string]map[string]int),
		foldedBodies: make(map[string]string),
		pins:         make(map[string]time.Time),
		pinsPath:     filepath.Join(root, ".notevault", "pins.json"),
	}
	if err := i.loadPins(); err != nil {
		return nil, err
	}
	return i, nil
}

func (i *memoryIndex) loadPins() error {
	raw, err := os.ReadFile(i.pinsPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lire les épingles : %w", err)
	}
	var stored pinsFile
	if err := json.Unmarshal(raw, &stored); err != nil {
		return fmt.Errorf("décoder les épingles : %w", err)
	}
	for _, pin := range stored.Pins {
		if pin.RelativePath != "" {
			i.pins[pin.RelativePath] = pin.PinnedAt.UTC()
		}
	}
	return nil
}

func (i *memoryIndex) savePinsLocked() error {
	pins := make([]pinRecord, 0, len(i.pins))
	for path, at := range i.pins {
		if _, exists := i.notes[path]; exists {
			pins = append(pins, pinRecord{RelativePath: path, PinnedAt: at})
		}
	}
	sort.Slice(pins, func(a, b int) bool {
		if pins[a].PinnedAt.Equal(pins[b].PinnedAt) {
			return pins[a].RelativePath < pins[b].RelativePath
		}
		return pins[a].PinnedAt.After(pins[b].PinnedAt)
	})
	clean := make(map[string]time.Time, len(pins))
	for _, pin := range pins {
		clean[pin.RelativePath] = pin.PinnedAt
	}
	i.pins = clean
	raw, err := json.MarshalIndent(pinsFile{Version: 1, Pins: pins}, "", "  ")
	if err != nil {
		return fmt.Errorf("encoder les épingles : %w", err)
	}
	if err := writeAtomic(i.pinsPath, raw, 0o600); err != nil {
		return fmt.Errorf("écrire les épingles : %w", err)
	}
	return nil
}

func tokenizeUnicode(value string) map[string]int {
	counts := make(map[string]int)
	token := make([]rune, 0, 24)
	flush := func() {
		if len(token) > 0 {
			counts[string(token)]++
			token = token[:0]
		}
	}
	for _, r := range foldUnicode(value) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || r == '_' {
			token = append(token, r)
		} else {
			flush()
		}
	}
	flush()
	return counts
}

func foldUnicode(value string) string {
	var folded strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(value)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		folded.WriteRune(r)
	}
	return folded.String()
}

func (i *memoryIndex) Upsert(note domain.Note) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.deleteTokensLocked(note.RelativePath)
	note.Tags = append([]string(nil), uniqueNonEmpty(note.Tags)...)
	i.notes[note.RelativePath] = note
	keys := tokenizeUnicode(note.Title + "\n" + note.Content)
	i.noteKeys[note.RelativePath] = keys
	i.foldedBodies[note.RelativePath] = foldUnicode(note.Title + "\n" + note.Content)
	for token, count := range keys {
		paths := i.tokens[token]
		if paths == nil {
			paths = make(map[string]int)
			i.tokens[token] = paths
		}
		paths[note.RelativePath] = count
	}
	return nil
}

func (i *memoryIndex) deleteTokensLocked(path string) {
	for token := range i.noteKeys[path] {
		delete(i.tokens[token], path)
		if len(i.tokens[token]) == 0 {
			delete(i.tokens, token)
		}
	}
	delete(i.noteKeys, path)
	delete(i.foldedBodies, path)
}

func (i *memoryIndex) Delete(path string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if _, ok := i.notes[path]; !ok {
		return ErrNotFound
	}
	i.deleteTokensLocked(path)
	delete(i.notes, path)
	if _, ok := i.pins[path]; ok {
		delete(i.pins, path)
		return i.savePinsLocked()
	}
	return nil
}

func (i *memoryIndex) Get(path string) (domain.Note, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	note, ok := i.notes[path]
	if !ok {
		return domain.Note{}, ErrNotFound
	}
	note.Tags = append([]string(nil), note.Tags...)
	return note, nil
}

func (i *memoryIndex) List(filter ListFilter) ([]domain.NoteSummary, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([]rankedSummary, 0, len(i.notes))
	paths := i.queryCandidatesLocked(filter.Query)
	if strings.TrimSpace(filter.Query) == "" {
		paths = make(map[string]struct{}, len(i.notes))
		for path := range i.notes {
			paths[path] = struct{}{}
		}
	}
	for path := range paths {
		note, exists := i.notes[path]
		if !exists {
			continue
		}
		if !matchesListFilter(note, filter) {
			continue
		}
		keys := i.noteKeys[path]
		folded := i.foldedBodies[path]
		score, ok := scoreQuery(note, keys, folded, filter.Query)
		if strings.TrimSpace(filter.Query) != "" && !ok {
			continue
		}
		out = append(out, rankedSummary{summary: summaryOf(note), score: score})
	}
	sort.SliceStable(out, func(a, b int) bool {
		if strings.TrimSpace(filter.Query) != "" && out[a].score != out[b].score {
			return out[a].score > out[b].score
		}
		if !out[a].summary.UpdatedAt.Equal(out[b].summary.UpdatedAt) {
			return out[a].summary.UpdatedAt.After(out[b].summary.UpdatedAt)
		}
		return out[a].summary.RelativePath < out[b].summary.RelativePath
	})
	return summaries(out, clampLimit(filter.Limit)), nil
}

func (i *memoryIndex) queryCandidatesLocked(query string) map[string]struct{} {
	parts := parseQueryParts(query)
	if len(parts) == 0 {
		all := make(map[string]struct{}, len(i.notes))
		for path := range i.notes {
			all[path] = struct{}{}
		}
		return all
	}
	var candidates map[string]struct{}
	for _, part := range parts {
		tokens := tokenizeUnicode(part.value)
		if len(tokens) == 0 {
			continue
		}
		// Every token of a phrase must exist. Exact adjacency is checked later.
		for token := range tokens {
			paths := i.tokens[token]
			if candidates == nil {
				candidates = make(map[string]struct{}, len(paths))
				for path := range paths {
					candidates[path] = struct{}{}
				}
				continue
			}
			for path := range candidates {
				if _, ok := paths[path]; !ok {
					delete(candidates, path)
				}
			}
		}
	}
	if candidates == nil {
		return map[string]struct{}{}
	}
	return candidates
}

func matchesListFilter(note domain.Note, filter ListFilter) bool {
	if folder := strings.Trim(strings.TrimSpace(filter.Folder), "/"); folder != "" {
		folder = strings.TrimPrefix(folder, "notes/")
		path := strings.TrimPrefix(note.RelativePath, "notes/")
		if path != folder && !strings.HasPrefix(path, folder+"/") {
			return false
		}
	}
	tags := make(map[string]struct{}, len(note.Tags))
	for _, tag := range note.Tags {
		tags[tag] = struct{}{}
	}
	required := append([]string{filter.Tag}, filter.Tags...)
	for _, tag := range required {
		if tag = strings.TrimSpace(tag); tag != "" {
			if _, ok := tags[tag]; !ok {
				return false
			}
		}
	}
	for _, tag := range filter.ExcludeTags {
		if _, ok := tags[strings.TrimSpace(tag)]; ok {
			return false
		}
	}
	if !filter.UpdatedFrom.IsZero() && note.UpdatedAt.Before(filter.UpdatedFrom) {
		return false
	}
	if !filter.UpdatedTo.IsZero() && !note.UpdatedAt.Before(filter.UpdatedTo) {
		return false
	}
	return true
}

type queryPart struct {
	value  string
	phrase bool
}

func parseQueryParts(query string) []queryPart {
	parts := make([]queryPart, 0)
	for query = strings.TrimSpace(query); query != ""; query = strings.TrimSpace(query) {
		if query[0] == '"' {
			query = query[1:]
			if end := strings.IndexByte(query, '"'); end >= 0 {
				if phrase := strings.TrimSpace(query[:end]); phrase != "" {
					parts = append(parts, queryPart{value: foldUnicode(phrase), phrase: true})
				}
				query = query[end+1:]
				continue
			}
		}
		end := strings.IndexAny(query, " \t\n")
		if end < 0 {
			end = len(query)
		}
		for token := range tokenizeUnicode(query[:end]) {
			parts = append(parts, queryPart{value: token})
		}
		query = query[end:]
	}
	return parts
}

// scoreQuery calcule le score d'un candidat en réutilisant les structures
// pré-construites à l'Upsert : keys est la tokenisation de Title+"\n"+Content,
// folded est le même texte plié Unicode. Seuls les dérivés du titre (titre
// plié, tokens du titre) sont recalculés par candidat car leur coût est
// proportionnel à la taille du titre et non du contenu.
func scoreQuery(note domain.Note, keys map[string]int, folded string, query string) (int, bool) {
	parts := parseQueryParts(query)
	if len(parts) == 0 {
		return 0, true
	}
	titleFolded := foldUnicode(note.Title)
	titleKeys := tokenizeUnicode(note.Title)
	score := 0
	for _, part := range parts {
		if part.phrase {
			if !strings.Contains(folded, part.value) {
				return 0, false
			}
			score += 8
			if strings.Contains(titleFolded, part.value) {
				score += 12
			}
			continue
		}
		count := keys[part.value]
		if count == 0 {
			return 0, false
		}
		score += count
		if titleKeys[part.value] > 0 {
			score += 10
		}
	}
	return score, true
}

func (i *memoryIndex) Search(query string, opts SearchOpts) ([]domain.NoteSummary, error) {
	return i.List(ListFilter{Query: query, Limit: opts.Limit})
}

func (i *memoryIndex) ListTags() ([]TagCount, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	counts := make(map[string]int)
	for _, note := range i.notes {
		for _, tag := range uniqueNonEmpty(note.Tags) {
			counts[tag]++
		}
	}
	return sortedTagCounts(counts, 0), nil
}

func (i *memoryIndex) ListFolders() ([]FolderInfo, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	counts := make(map[string]int)
	all := make(map[string]struct{})
	for path := range i.notes {
		dir := strings.TrimPrefix(filepath.ToSlash(filepath.Dir(path)), "notes/")
		if dir == "." || dir == "notes" || dir == "" {
			continue
		}
		counts[dir]++
		for parent := dir; parent != "." && parent != ""; parent = filepath.ToSlash(filepath.Dir(parent)) {
			all[parent] = struct{}{}
		}
	}
	out := make([]FolderInfo, 0, len(all))
	for path := range all {
		out = append(out, FolderInfo{Path: path, Name: filepath.Base(path), Count: counts[path]})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Path < out[b].Path })
	return out, nil
}

func (i *memoryIndex) Pin(path string, pinned bool) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if pinned {
		if _, ok := i.notes[path]; !ok {
			return ErrNotFound
		}
		i.pins[path] = nowUTC()
	} else {
		delete(i.pins, path)
	}
	return i.savePinsLocked()
}

func (i *memoryIndex) IsPinned(path string) (bool, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	_, ok := i.pins[path]
	return ok, nil
}

func (i *memoryIndex) ListPinned() ([]domain.NoteSummary, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	type pinnedNote struct {
		note domain.NoteSummary
		at   time.Time
	}
	out := make([]pinnedNote, 0, len(i.pins))
	dirty := false
	for path, at := range i.pins {
		note, ok := i.notes[path]
		if !ok {
			delete(i.pins, path)
			dirty = true
			continue
		}
		out = append(out, pinnedNote{note: summaryOf(note), at: at})
	}
	if dirty {
		if err := i.savePinsLocked(); err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(a, b int) bool {
		return out[a].at.After(out[b].at) || out[a].at.Equal(out[b].at) && out[a].note.RelativePath < out[b].note.RelativePath
	})
	result := make([]domain.NoteSummary, len(out))
	for n := range out {
		result[n] = out[n].note
	}
	return result, nil
}

func (i *memoryIndex) GetBacklinks(title string, opts SearchOpts) ([]domain.NoteSummary, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	title = strings.TrimSpace(title)
	if title == "" {
		return []domain.NoteSummary{}, nil
	}
	foldedTitle := foldUnicode(title)
	out := make([]rankedSummary, 0)
	for path := range i.queryCandidatesLocked(`"` + foldedTitle + `"`) {
		if path == opts.ExcludePath {
			continue
		}
		note, exists := i.notes[path]
		if !exists {
			continue
		}
		if count := countWikiLinksTo(note.Content, title); count > 0 {
			out = append(out, rankedSummary{summary: summaryOf(note), score: count})
		}
	}
	sort.Slice(out, func(a, b int) bool {
		return out[a].score > out[b].score || out[a].score == out[b].score && out[a].summary.UpdatedAt.After(out[b].summary.UpdatedAt)
	})
	return summaries(out, clampLimit(opts.Limit)), nil
}

func countWikiLinksTo(content, title string) int {
	count := 0
	for _, match := range wikiLinkPattern.FindAllStringSubmatch(content, -1) {
		if match[1] == title {
			count++
		}
	}
	return count
}

func (i *memoryIndex) StatsBuckets(windowDays int) (StatsBucketsResult, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if windowDays <= 0 {
		windowDays = 30
	}
	now := nowUTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -windowDays+1)
	created, modified, tags := make(map[string]int), make(map[string]int), make(map[string]int)
	words := 0
	for _, note := range i.notes {
		if !note.CreatedAt.Before(start) {
			created[note.CreatedAt.UTC().Format("2006-01-02")]++
		}
		if !note.UpdatedAt.Before(start) {
			modified[note.UpdatedAt.UTC().Format("2006-01-02")]++
		}
		words += countWords(note.Content)
		for _, tag := range uniqueNonEmpty(note.Tags) {
			tags[tag]++
		}
	}
	return StatsBucketsResult{Created: sortedDayCounts(created), Modified: sortedDayCounts(modified), Notes: len(i.notes), Words: words, TopTags: sortedTagCounts(tags, 10)}, nil
}

func sortedDayCounts(counts map[string]int) []DayCount {
	out := make([]DayCount, 0, len(counts))
	for day, count := range counts {
		out = append(out, DayCount{Day: day, Count: count})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Day < out[b].Day })
	return out
}

func sortedTagCounts(counts map[string]int, limit int) []TagCount {
	out := make([]TagCount, 0, len(counts))
	for tag, count := range counts {
		out = append(out, TagCount{Tag: tag, Count: count})
	}
	sort.Slice(out, func(a, b int) bool {
		return out[a].Count > out[b].Count || out[a].Count == out[b].Count && out[a].Tag < out[b].Tag
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (i *memoryIndex) ListPaths() ([]string, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([]string, 0, len(i.notes))
	for path := range i.notes {
		out = append(out, path)
	}
	sort.Strings(out)
	return out, nil
}

func (i *memoryIndex) SetMeta(string, string) error { return nil }

func (i *memoryIndex) reset() {
	i.mu.Lock()
	defer i.mu.Unlock()
	for path, note := range i.notes {
		note.Content = ""
		i.notes[path] = note
	}
	i.notes = make(map[string]domain.Note)
	i.tokens = make(map[string]map[string]int)
	i.noteKeys = make(map[string]map[string]int)
	i.foldedBodies = make(map[string]string)
}

func (i *memoryIndex) Close() error {
	i.reset()
	return nil
}

func summaryOf(note domain.Note) domain.NoteSummary {
	return domain.NoteSummary{
		RelativePath: note.RelativePath,
		Title:        note.Title,
		UpdatedAt:    note.UpdatedAt,
		Tags:         append([]string(nil), note.Tags...),
	}
}

func summaries(ranked []rankedSummary, limit int) []domain.NoteSummary {
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]domain.NoteSummary, len(ranked))
	for n := range ranked {
		out[n] = ranked[n].summary
	}
	return out
}

func clampLimit(n int) int {
	if n <= 0 {
		return 1000
	}
	if n > 5000 {
		return 5000
	}
	return n
}

func uniqueNonEmpty(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
