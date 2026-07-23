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
