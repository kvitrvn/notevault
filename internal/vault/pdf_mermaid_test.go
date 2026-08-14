package vault

import (
	"fmt"
	"strings"
	"testing"
)

// Fixture partagée avec frontend/src/lib/mermaid-blocks.test.ts : les deux
// implémentations doivent produire la même empreinte, sinon aucun diagramme ne
// sera jamais associé à son bloc.
const (
	mermaidFixtureCode = "flowchart TD\n  A[Début] --> B{Choix}\n  B --> C[Fin]"
	mermaidFixtureHash = "069b1401af8eeeca1a2947183f2101712ba44d0e2e2acc9792b74e0f19c2a239"
)

func validTestSVG(label string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" width="120" height="60" viewBox="0 0 120 60">` +
		`<text x="4" y="20">` + label + `</text></svg>`
}

func TestMermaidCodeHashMatchesFrontendContract(t *testing.T) {
	if got := mermaidCodeHash(mermaidFixtureCode); got != mermaidFixtureHash {
		t.Fatalf("hash = %q, want %q", got, mermaidFixtureHash)
	}
	// Le corps issu de goldmark porte le saut de ligne final du bloc et
	// éventuellement des fins de ligne Windows : l'empreinte doit être stable.
	for _, variant := range []string{
		mermaidFixtureCode + "\n",
		strings.ReplaceAll(mermaidFixtureCode, "\n", "\r\n") + "\r\n",
	} {
		if got := mermaidCodeHash(variant); got != mermaidFixtureHash {
			t.Errorf("hash(%q) = %q, want %q", variant, got, mermaidFixtureHash)
		}
	}
}

func TestBuildNotePDFDocumentEmbedsPreRenderedMermaid(t *testing.T) {
	service := newPDFTestService(t)
	note := createPDFTestNote(t, service, "Flux", strings.Join([]string{
		"```mermaid",
		mermaidFixtureCode,
		"```",
	}, "\n"))

	document, err := service.BuildNotePDFDocument(note.RelativePath, "classic", false, map[string]string{
		mermaidFixtureHash: validTestSVG("Début"),
	})
	if err != nil {
		t.Fatal(err)
	}
	html := string(document.HTML)
	if !strings.Contains(html, `<img class="mermaid-diagram" src="data:image/svg+xml;base64,`) {
		t.Fatalf("pre-rendered diagram was not embedded:\n%s", html)
	}
	if strings.Contains(html, `class="language-mermaid"`) {
		t.Error("the code block was kept alongside the diagram")
	}
	// Le SVG doit rester encodé : rien d'inline dans le document imprimé.
	if strings.Contains(html, "<svg") {
		t.Error("inline SVG was emitted")
	}
}

func TestBuildNotePDFDocumentFallsBackToCodeBlock(t *testing.T) {
	service := newPDFTestService(t)
	note := createPDFTestNote(t, service, "Flux", strings.Join([]string{
		"```mermaid",
		mermaidFixtureCode,
		"```",
	}, "\n"))

	cases := map[string]map[string]string{
		"aucun diagramme":     nil,
		"empreinte inconnue":  {strings.Repeat("a", 64): validTestSVG("X")},
		"empreinte invalide":  {"pas-une-empreinte": validTestSVG("X")},
		"SVG mal formé":       {mermaidFixtureHash: `<svg xmlns="http://www.w3.org/2000/svg"><text>`},
		"racine non SVG":      {mermaidFixtureHash: `<html xmlns="http://www.w3.org/2000/svg"></html>`},
		"sans espace de noms": {mermaidFixtureHash: `<svg width="10" height="10"></svg>`},
		"SVG trop volumineux": {
			mermaidFixtureHash: `<svg xmlns="http://www.w3.org/2000/svg"><!--` +
				strings.Repeat("x", maxMermaidSVGBytes) + `--></svg>`,
		},
	}
	for name, diagrams := range cases {
		t.Run(name, func(t *testing.T) {
			document, err := service.BuildNotePDFDocument(note.RelativePath, "classic", false, diagrams)
			if err != nil {
				t.Fatal(err)
			}
			html := string(document.HTML)
			if !strings.Contains(html, `class="language-mermaid"`) {
				t.Errorf("code block fallback missing:\n%s", html)
			}
			if strings.Contains(html, `<img class="mermaid-diagram"`) {
				t.Error("an invalid diagram was embedded")
			}
		})
	}
}

func TestPrepareMermaidDiagramsEnforcesBudgets(t *testing.T) {
	tooMany := make(map[string]string, maxMermaidDiagrams+1)
	for index := 0; index <= maxMermaidDiagrams; index++ {
		tooMany[fmt.Sprintf("%064x", index)] = validTestSVG("X")
	}
	if got := prepareMermaidDiagrams(tooMany); got != nil {
		t.Errorf("a payload over the diagram cap was accepted (%d entries)", len(got))
	}

	accepted := prepareMermaidDiagrams(map[string]string{mermaidFixtureHash: validTestSVG("Début")})
	if len(accepted) != 1 {
		t.Fatalf("valid diagram was rejected: %v", accepted)
	}
	if !strings.HasPrefix(accepted[mermaidFixtureHash], "data:image/svg+xml;base64,") {
		t.Errorf("unexpected data URI: %q", accepted[mermaidFixtureHash])
	}
}

func TestPrepareMermaidDiagramsStopsAtTotalBudget(t *testing.T) {
	// Chaque entrée reste sous la limite unitaire, mais leur somme dépasse le
	// budget du document : les dernières sont écartées, dans un ordre stable
	// qui ne dépend pas de l'itération de la map.
	wrapper := len(`<svg xmlns="http://www.w3.org/2000/svg"><!----></svg>`)
	big := `<svg xmlns="http://www.w3.org/2000/svg"><!--` +
		strings.Repeat("x", maxMermaidSVGBytes-wrapper) + `--></svg>`
	diagrams := make(map[string]string, 9)
	for index := 0; index < 9; index++ {
		diagrams[fmt.Sprintf("%064x", index)] = big
	}
	if got := len(prepareMermaidDiagrams(diagrams)); got != maxMermaidTotalBytes/maxMermaidSVGBytes {
		t.Fatalf("kept %d diagrams, want %d", got, maxMermaidTotalBytes/maxMermaidSVGBytes)
	}
}
