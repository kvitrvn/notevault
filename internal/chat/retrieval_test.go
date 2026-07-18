package chat

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestRetrieveRanksRelevantMarkdownSection(t *testing.T) {
	t.Parallel()
	notes := []Note{
		{Path: "notes/cuisine.md", Title: "Cuisine", Content: "# Cuisine\n\nLa pâte repose une heure."},
		{Path: "notes/go.md", Title: "Go", Content: "# Go\n\n## Concurrence\n\nLes goroutines communiquent par channels.\n\n## Tests\n\nLes tests utilisent testing."},
	}

	got, err := retrieve(context.Background(), notes, "comment communiquent les goroutines")
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("retrieve n'a renvoyé aucun passage")
	}
	if got[0].Path != "notes/go.md" {
		t.Fatalf("première source = %q, want notes/go.md", got[0].Path)
	}
	if got[0].Section != "Concurrence" && got[0].Section != "Go" {
		t.Fatalf("section = %q, want Concurrence ou Go", got[0].Section)
	}
}

func TestRetrieveFallsBackForQuestionWithoutLexicalMatch(t *testing.T) {
	t.Parallel()
	notes := make([]Note, 0, maxExcerpts+1)
	for index := 0; index < maxExcerpts+1; index++ {
		notes = append(notes, Note{
			Path:    fmt.Sprintf("notes/%d.md", index),
			Title:   fmt.Sprintf("Note %d", index),
			Content: fmt.Sprintf("# Journal %d\n\nUne pensée sans mot clé.\n\n## Suite\n\nAutre passage.", index),
		})
	}

	got, err := retrieve(context.Background(), notes, "zzzz-introuvable")
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(got) != maxExcerpts {
		t.Fatalf("len(fallback) = %d, want %d", len(got), maxExcerpts)
	}
	seen := make(map[string]struct{}, len(got))
	for _, excerpt := range got {
		if _, duplicate := seen[excerpt.Path]; duplicate {
			t.Fatalf("le repli contient plusieurs passages de %q : %+v", excerpt.Path, got)
		}
		seen[excerpt.Path] = struct{}{}
	}
}

func TestRetrieveDoesNotIncludeNestedSectionsInParentPassage(t *testing.T) {
	t.Parallel()
	notes := []Note{{
		Path:  "notes/aster.md",
		Title: "Aster",
		Content: "# Projet Aster\n\nIntroduction générale.\n\n" +
			"## RGPD\n\n" + strings.Repeat("La conservation des données est strictement limitée.\n\n", 24) +
			"## Budget\n\nCONFIDENTIEL_BUDGET_INUTILE ne concerne pas la demande.\n",
	}}

	got, err := retrieve(context.Background(), notes, "conservation données")
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("retrieve n'a renvoyé aucun passage")
	}
	for _, excerpt := range got {
		if strings.Contains(excerpt.Content, "CONFIDENTIEL_BUDGET_INUTILE") {
			t.Fatalf("le passage pertinent contient une sous-section hors sujet : %q", excerpt.Content)
		}
	}
}

func TestSplitMarkdownPassagesUsesRealHeadingsOnly(t *testing.T) {
	t.Parallel()
	passages, err := splitMarkdownPassages([]byte("Titre principal\n===============\n\nIntroduction.\n\n```md\n# Faux titre\n```\n\n## Suite\n\nContenu utile."))
	if err != nil {
		t.Fatalf("splitMarkdownPassages: %v", err)
	}
	if len(passages) != 2 {
		t.Fatalf("len(passages) = %d, want 2: %+v", len(passages), passages)
	}
	if passages[0].label != "Titre principal" || passages[1].label != "Suite" {
		t.Fatalf("labels = %q, %q", passages[0].label, passages[1].label)
	}
}

func TestSplitByWordsCapsPassageSize(t *testing.T) {
	t.Parallel()
	chunks := splitByWords(strings.Repeat("mot ", maxExcerptWords+1), maxExcerptWords)
	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, want 2", len(chunks))
	}
}
