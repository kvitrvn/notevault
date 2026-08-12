package vault

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kvitrvn/notevault/internal/domain"
)

const validPDFThemeJSON = `{
  "version": 1,
  "page": {
    "size": "A4",
    "orientation": "portrait",
    "margins": {"top": 15, "right": 16, "bottom": 17, "left": 18}
  },
  "typography": {
    "family": "sans-serif",
    "monoFamily": "monospace",
    "bodySizePt": 11,
    "lineHeight": 1.5,
    "headingScale": 1.25
  },
  "colors": {
    "text": "#202124",
    "secondary": "#5f6368",
    "accent": "#315c8c",
    "codeBackground": "#f3f4f6"
  },
  "options": {"titlePage": true, "metadata": true, "pageNumbers": true}
}`

const validPDFThemeStylesheet = `
body { font-family: "Geist", sans-serif; }
pre, code { font-family: "Geist Mono", monospace; }
blockquote { border-left: 3pt solid #0a5bd4; }
@media print { a { text-decoration: none; } }
`

func TestListPDFThemesIncludesClassicAndWarnsForInvalidCustomThemes(t *testing.T) {
	service := newPDFTestService(t)
	dir := filepath.Join(service.root, ".notevault", "pdf-themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, "report.json"), []byte(validPDFThemeJSON))
	writeTestFile(t, filepath.Join(dir, "unknown.json"), []byte(strings.Replace(
		validPDFThemeJSON, `"version": 1,`, `"version": 1, "css": "url(https://example.test)",`, 1,
	)))
	writeTestFile(t, filepath.Join(dir, "classic.json"), []byte(validPDFThemeJSON))

	themes, warnings := service.ListPDFThemes()
	if len(themes) != 2 || themes[0].ID != "classic" || themes[1].ID != "report" {
		t.Fatalf("themes = %+v", themes)
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %v, want 2", warnings)
	}
}

func TestListPDFThemesSupportsStandaloneAndCompanionStylesheets(t *testing.T) {
	service := newPDFTestService(t)
	dir := filepath.Join(service.root, ".notevault", "pdf-themes")
	writeTestFile(t, filepath.Join(dir, "beewii.css"), []byte(validPDFThemeStylesheet))
	writeTestFile(t, filepath.Join(dir, "beewii.html"), []byte(`<script>alert("ignored")</script>`))
	writeTestFile(t, filepath.Join(dir, "report.json"), []byte(validPDFThemeJSON))
	writeTestFile(t, filepath.Join(dir, "report.css"), []byte("main { max-width: 46rem; }"))

	themes, warnings := service.ListPDFThemes()
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if len(themes) != 3 || themes[0].ID != "classic" || themes[1].ID != "beewii" || themes[2].ID != "report" {
		t.Fatalf("themes = %+v", themes)
	}
	if themes[1].Builtin || themes[1].stylesheet != validPDFThemeStylesheet {
		t.Fatalf("standalone CSS theme = %+v", themes[1])
	}
	if themes[1].Page.Margins != classicPDFTheme().Page.Margins {
		t.Fatalf("standalone CSS margins = %+v", themes[1].Page.Margins)
	}
	if themes[2].Page.Margins.Top != 15 || themes[2].stylesheet != "main { max-width: 46rem; }" {
		t.Fatalf("JSON + CSS theme = %+v", themes[2])
	}
}

func TestListPDFThemesRejectsStylesheetSymlinks(t *testing.T) {
	service := newPDFTestService(t)
	dir := filepath.Join(service.root, ".notevault", "pdf-themes")
	target := filepath.Join(t.TempDir(), "outside.css")
	writeTestFile(t, target, []byte(validPDFThemeStylesheet))
	if err := os.Symlink(target, filepath.Join(dir, "outside.css")); err != nil {
		t.Fatal(err)
	}

	themes, warnings := service.ListPDFThemes()
	if len(themes) != 1 || themes[0].ID != "classic" {
		t.Fatalf("themes = %+v", themes)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "liens symboliques") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestLoadPDFThemeRejectsInvalidData(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{name: "unknown field", json: strings.Replace(validPDFThemeJSON, `"version": 1,`, `"version": 1, "html": "<script>",`, 1)},
		{name: "identity field", json: strings.Replace(validPDFThemeJSON, `"version": 1,`, `"version": 1, "id": "forged",`, 1)},
		{name: "trailing JSON", json: validPDFThemeJSON + `{}`},
		{name: "margin too small", json: strings.Replace(validPDFThemeJSON, `"top": 15`, `"top": 4`, 1)},
		{name: "margin too large", json: strings.Replace(validPDFThemeJSON, `"left": 18`, `"left": 41`, 1)},
		{name: "body too small", json: strings.Replace(validPDFThemeJSON, `"bodySizePt": 11`, `"bodySizePt": 8`, 1)},
		{name: "line height too large", json: strings.Replace(validPDFThemeJSON, `"lineHeight": 1.5`, `"lineHeight": 2.1`, 1)},
		{name: "heading scale too large", json: strings.Replace(validPDFThemeJSON, `"headingScale": 1.25`, `"headingScale": 2.1`, 1)},
		{name: "css injection", json: strings.Replace(validPDFThemeJSON, `"#315c8c"`, `"red;url(https://example.test)"`, 1)},
		{name: "html injection", json: strings.Replace(validPDFThemeJSON, `"sans-serif"`, `"<style>"`, 1)},
		{name: "font url", json: strings.Replace(validPDFThemeJSON, `"monospace"`, `"url(file:///tmp/font)"`, 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "theme.json")
			writeTestFile(t, path, []byte(test.json))
			if _, err := loadPDFThemeFile(path, "theme"); err == nil {
				t.Fatal("loadPDFThemeFile succeeded")
			}
		})
	}
}

func TestLoadPDFThemeRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.json")
	writeTestFile(t, path, bytes.Repeat([]byte(" "), maxPDFThemeBytes+1))
	if _, err := loadPDFThemeFile(path, "large"); err == nil || !strings.Contains(err.Error(), "64 Kio") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadPDFThemeStylesheetRejectsUnsafeCSS(t *testing.T) {
	tests := []struct {
		name string
		css  []byte
	}{
		{name: "remote URL", css: []byte(`body { background: url(https://example.test/a.png); }`)},
		{name: "obfuscated import", css: []byte(`@im/**/port "theme.css";`)},
		{name: "font face", css: []byte(`@font-face { src: local("Geist"); }`)},
		{name: "HTML escape", css: []byte(`</style><script>alert(1)</script>`)},
		{name: "CSS escape", css: []byte(`body { background: u\72l(example.test); }`)},
		{name: "unterminated comment", css: []byte(`body {} /*`)},
		{name: "invalid UTF-8", css: []byte{0xff}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "theme.css")
			writeTestFile(t, path, test.css)
			if _, err := loadPDFThemeStylesheet(path); err == nil {
				t.Fatal("loadPDFThemeStylesheet succeeded")
			}
		})
	}
}

func TestLoadPDFThemeStylesheetRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.css")
	writeTestFile(t, path, bytes.Repeat([]byte(" "), maxPDFStylesheetBytes+1))
	if _, err := loadPDFThemeStylesheet(path); err == nil || !strings.Contains(err.Error(), "64 Kio") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildNotePDFDocumentUsesLocalCSSTheme(t *testing.T) {
	service := newPDFTestService(t)
	dir := filepath.Join(service.root, ".notevault", "pdf-themes")
	stylesheet := validPDFThemeStylesheet + `
header { background: linear-gradient(150deg, #062a78 0%, #0a49ab 45%, #0a6fde 100%); }
pre { background: #f6f8fb; }
`
	writeTestFile(t, filepath.Join(dir, "beewii.css"), []byte(stylesheet))
	note := createPDFTestNote(t, service, "Audit de conformité", strings.Join([]string{
		"## Synthèse",
		"",
		"> Les traitements restent maîtrisés.",
		"",
		"| Contrôle | Statut |",
		"| --- | --- |",
		"| Chiffrement | Conforme |",
		"",
		"```go",
		`fmt.Println("auditable")`,
		"```",
	}, "\n"))

	document, err := service.BuildNotePDFDocument(note.RelativePath, "beewii", false)
	if err != nil {
		t.Fatal(err)
	}
	html := string(document.HTML)
	for _, expected := range []string{
		`linear-gradient(150deg, #062a78 0%, #0a49ab 45%, #0a6fde 100%)`,
		`font-family: "Geist",`,
		`font-family: "Geist Mono",`,
		`border-left: 3pt solid #0a5bd4`,
		`background: #f6f8fb`,
		`font-src 'none'`,
	} {
		if !strings.Contains(html, expected) {
			t.Errorf("local CSS theme HTML does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{"fonts.googleapis.com", "https://", "file://"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("local CSS theme HTML contains remote/local resource %q", forbidden)
		}
	}
	if document.Margins != classicPDFTheme().Page.Margins {
		t.Fatalf("margins = %+v", document.Margins)
	}
	if !document.PageNumbers {
		t.Fatal("standalone CSS theme must inherit page numbers")
	}
}

func TestBuildNotePDFDocumentRendersGFMAndEscapesRawHTML(t *testing.T) {
	service := newPDFTestService(t)
	note := createPDFTestNote(t, service, "Rapport", strings.Join([]string{
		"<script>window.evil = true</script>",
		"",
		"| Colonne | Valeur |",
		"| --- | --- |",
		"| A | B |",
		"",
		"- [x] Terminé",
		"",
		"```mermaid",
		"graph TD; A-->B",
		"```",
	}, "\n"))

	document, err := service.BuildNotePDFDocument(note.RelativePath, "classic", false)
	if err != nil {
		t.Fatal(err)
	}
	html := string(document.HTML)
	for _, expected := range []string{
		`default-src 'none'`,
		"<table>",
		`type="checkbox"`,
		`class="language-mermaid"`,
		"&lt;script&gt;window.evil = true&lt;/script&gt;",
	} {
		if !strings.Contains(html, expected) {
			t.Errorf("HTML does not contain %q:\n%s", expected, html)
		}
	}
	if strings.Contains(html, "<script>") {
		t.Fatal("raw script was emitted")
	}
}

func TestBuildNotePDFDocumentEmbedsOnlyValidatedRasterAssets(t *testing.T) {
	service := newPDFTestService(t)
	pngData, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
	)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(service.root, "assets", "valid.png"), pngData)
	writeTestFile(t, filepath.Join(service.root, "assets", "vector.svg"), []byte(`<svg><script/></svg>`))
	outside := filepath.Join(t.TempDir(), "outside.png")
	writeTestFile(t, outside, pngData)
	if err := os.Symlink(outside, filepath.Join(service.root, "assets", "linked.png")); err != nil {
		t.Fatal(err)
	}
	note := createPDFTestNote(t, service, "Images", strings.Join([]string{
		"![Valide](assets/valid.png)",
		"![Distante](https://example.test/tracker.png)",
		"![Traversal](assets/../../outside.png)",
		"![Lien](assets/linked.png)",
		"![Vecteur](assets/vector.svg)",
	}, "\n\n"))

	document, err := service.BuildNotePDFDocument(note.RelativePath, "classic", false)
	if err != nil {
		t.Fatal(err)
	}
	html := string(document.HTML)
	if !strings.Contains(html, `src="data:image/png;base64,`) {
		t.Fatal("valid PNG was not embedded")
	}
	for _, forbidden := range []string{"https://example.test", "../../outside.png", "assets/linked.png", "<svg"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("HTML contains forbidden value %q", forbidden)
		}
	}
	for _, alt := range []string{"Distante", "Traversal", "Lien", "Vecteur"} {
		if !strings.Contains(html, "[Image : "+alt+"]") {
			t.Errorf("missing fallback for %q", alt)
		}
	}
}

func TestBuildNotePDFDocumentRequiresPlaintextConfirmationForEncryptedVault(t *testing.T) {
	service := newPDFTestService(t)
	note := createPDFTestNote(t, service, "Secret", "Contenu")
	if err := service.EnableEncryption("phrase secrète assez longue"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.BuildNotePDFDocument(note.RelativePath, "classic", false); err == nil {
		t.Fatal("encrypted export succeeded without confirmation")
	}
	if _, err := service.BuildNotePDFDocument(note.RelativePath, "classic", true); err != nil {
		t.Fatalf("confirmed encrypted export failed: %v", err)
	}
}

func TestWritePDFAtomicValidatesSignatureAndDoesNotLeavePartialOutput(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "note.pdf")
	if err := WritePDFAtomic(destination, []byte("not a pdf")); err == nil {
		t.Fatal("invalid output was accepted")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination exists after failure: %v", err)
	}
	want := []byte("%PDF-1.7\nbody")
	if err := WritePDFAtomic(destination, want); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func newPDFTestService(t *testing.T) *Service {
	t.Helper()
	service, err := New(Options{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	return service
}

func createPDFTestNote(t *testing.T, service *Service, title, content string) domain.Note {
	t.Helper()
	note, err := service.CreateNote("", title, "")
	if err != nil {
		t.Fatal(err)
	}
	note.Content = content
	note, err = service.SaveNote(note)
	if err != nil {
		t.Fatal(err)
	}
	return note
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
