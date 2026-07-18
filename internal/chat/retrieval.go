package chat

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"

	bleve "github.com/blevesearch/bleve/v2"
	amoxtliIndex "github.com/bornholm/amoxtli/index"
	amoxtliBleve "github.com/bornholm/amoxtli/index/bleve"
	amoxtliMarkdown "github.com/bornholm/amoxtli/markdown"
	"github.com/bornholm/amoxtli/model"
	"github.com/yuin/goldmark/ast"
	goldmarkText "github.com/yuin/goldmark/text"
)

type retrievedExcerpt struct {
	Path      string
	Title     string
	Section   string
	Content   string
	sectionID model.SectionID
}

func retrieve(ctx context.Context, notes []Note, query string) ([]retrievedExcerpt, error) {
	raw, err := bleve.NewMemOnly(amoxtliBleve.IndexMapping())
	if err != nil {
		return nil, fmt.Errorf("créer l’index de chat en mémoire : %w", err)
	}
	idx := amoxtliBleve.NewIndex(raw)
	defer idx.Close()

	sections := make(map[model.SectionID]retrievedExcerpt)
	sourceNotes := make(map[string]Note, len(notes))
	fallback := make([]retrievedExcerpt, 0, min(len(notes), maxExcerpts))

	for noteIndex, note := range notes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		passages, err := splitMarkdownPassages([]byte(note.Content))
		if err != nil {
			return nil, fmt.Errorf("découper %q : %w", note.Path, err)
		}
		source := &url.URL{Scheme: "notevault", Host: "note", Path: fmt.Sprintf("/%d", noteIndex)}
		sourceNotes[source.String()] = note
		fallbackAdded := false

		for _, passage := range passages {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			sectionID := model.NewSectionID()
			excerpt := retrievedExcerpt{
				Path:      note.Path,
				Title:     note.Title,
				Section:   passage.label,
				Content:   passage.content,
				sectionID: sectionID,
			}
			sections[sectionID] = excerpt
			if !fallbackAdded && len(fallback) < maxExcerpts {
				fallback = append(fallback, excerpt)
				fallbackAdded = true
			}
			if err := raw.Index(string(sectionID), map[string]any{
				"_type":   "resource",
				"content": passage.content,
				"source":  source.String(),
			}); err != nil {
				return nil, fmt.Errorf("indexer une section de %q pour le chat : %w", note.Path, err)
			}
		}
	}

	results, err := idx.Search(ctx, query, amoxtliIndex.SearchOptions{MaxResults: maxExcerpts})
	if err != nil {
		return nil, fmt.Errorf("chercher dans la sélection : %w", err)
	}

	out := make([]retrievedExcerpt, 0, maxExcerpts)
	seen := make(map[model.SectionID]struct{})
	for _, result := range results {
		if result.Source == nil {
			continue
		}
		note, exists := sourceNotes[result.Source.String()]
		if !exists {
			continue
		}
		ids := append([]model.SectionID(nil), result.Sections...)
		sort.SliceStable(ids, func(i, j int) bool {
			return result.SectionScores[ids[i]] > result.SectionScores[ids[j]]
		})
		for _, id := range ids {
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			excerpt, ok := sections[id]
			if !ok {
				continue
			}
			excerpt.Path = note.Path
			excerpt.Title = note.Title
			out = append(out, excerpt)
			seen[id] = struct{}{}
			if len(out) == maxExcerpts {
				return out, nil
			}
		}
	}
	if len(out) == 0 {
		return fallback, nil
	}
	return out, nil
}

type markdownPassage struct {
	label   string
	content string
}

// splitMarkdownPassages keeps heading sections independent before Amoxtli's
// lexical index sees them. The upstream Markdown section model aggregates
// descendants into their parents, which is useful for navigation but too
// broad for a privacy-sensitive RAG prompt.
func splitMarkdownPassages(markdown []byte) ([]markdownPassage, error) {
	starts := []int{0}
	root := amoxtliMarkdown.New().Parser().Parse(goldmarkText.NewReader(markdown))
	if err := ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := node.(*ast.Heading)
		if !ok || heading.Lines().Len() == 0 {
			return ast.WalkContinue, nil
		}
		contentStart := heading.Lines().At(0).Start
		start := bytes.LastIndexByte(markdown[:contentStart], '\n') + 1
		if start > starts[len(starts)-1] {
			starts = append(starts, start)
		}
		return ast.WalkContinue, nil
	}); err != nil {
		return nil, err
	}

	passages := make([]markdownPassage, 0, len(starts))
	for index, start := range starts {
		end := len(markdown)
		if index+1 < len(starts) {
			end = starts[index+1]
		}
		trimmed, err := amoxtliMarkdown.Trim(markdown[start:end])
		if err != nil {
			return nil, err
		}
		content := strings.TrimSpace(string(trimmed))
		if content == "" {
			continue
		}
		label := sectionLabel(content)
		for _, chunk := range splitByWords(content, maxExcerptWords) {
			passages = append(passages, markdownPassage{label: label, content: chunk})
		}
	}
	return passages, nil
}

func splitByWords(content string, maximum int) []string {
	if maximum <= 0 || strings.TrimSpace(content) == "" {
		return nil
	}
	chunks := make([]string, 0, 1)
	start := 0
	words := 0
	inWord := false
	for index, character := range content {
		separator := unicode.IsSpace(character) || unicode.IsPunct(character)
		if !separator && !inWord {
			if words == maximum {
				if chunk := strings.TrimSpace(content[start:index]); chunk != "" {
					chunks = append(chunks, chunk)
				}
				start = index
				words = 0
			}
			words++
		}
		inWord = !separator
	}
	if chunk := strings.TrimSpace(content[start:]); chunk != "" {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func sectionLabel(content string) string {
	line := strings.TrimSpace(strings.SplitN(content, "\n", 2)[0])
	line = strings.TrimSpace(strings.TrimLeft(line, "#"))
	if line == "" {
		return "Début de la note"
	}
	if len([]rune(line)) > 100 {
		return string([]rune(line)[:100]) + "…"
	}
	return line
}
