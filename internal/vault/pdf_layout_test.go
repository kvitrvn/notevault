package vault

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const validPDFThemeManifest = `{
  "version": 2,
  "name": "Dummy Package",
  "layout": "document.html",
  "page": {
    "size": "A4",
    "orientation": "portrait",
    "margins": {"top": 15, "right": 16, "bottom": 17, "left": 18}
  },
  "options": {"pageNumbers": true},
  "vars": {"footerText": "Dummy"}
}`

const validPDFThemeLayout = `<!doctype html>
<html lang="fr">
  <head>
    <meta charset="utf-8" />
    <title>{{ default ( get .Vars "footerText" ) ( get .Meta "title" ) }}</title>
    <style>body { font-family: "Geist", sans-serif; }</style>
  </head>
  <body class="markdown-body">
    <footer>{{ get .Vars "footerText" }}</footer>
    {{ .Body }}
  </body>
</html>
`

// pdfTestPNG is a 1x1 opaque PNG, small enough to inline as a fixture.
const pdfTestPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

func writePDFThemePackage(t *testing.T, service *Service, id string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(service.root, ".notevault", "pdf-themes", id)
	for name, content := range files {
		writeTestFile(t, filepath.Join(dir, filepath.FromSlash(name)), []byte(content))
	}
	return dir
}

func findPDFTheme(themes []PDFTheme, id string) (PDFTheme, bool) {
	for _, theme := range themes {
		if theme.ID == id {
			return theme, true
		}
	}
	return PDFTheme{}, false
}

func TestListPDFThemesDiscoversPackageThemes(t *testing.T) {
	service := newPDFTestService(t)
	writePDFThemePackage(t, service, "dummy-package", map[string]string{
		pdfThemeManifestName: validPDFThemeManifest,
		"document.html":      validPDFThemeLayout,
	})

	themes, warnings := service.ListPDFThemes()
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	theme, ok := findPDFTheme(themes, "dummy-package")
	if !ok {
		t.Fatal("package theme was not listed")
	}
	if theme.Name != "Dummy Package" {
		t.Errorf("name = %q, want the manifest name", theme.Name)
	}
	if theme.Builtin || theme.Version != 2 {
		t.Errorf("unexpected builtin/version: %v/%d", theme.Builtin, theme.Version)
	}
	if theme.layoutFile != "document.html" {
		t.Errorf("layoutFile = %q", theme.layoutFile)
	}
	if theme.Page.Margins.Top != 15 || theme.Page.Margins.Left != 18 {
		t.Errorf("margins not taken from the manifest: %+v", theme.Page.Margins)
	}
	if !theme.Options.PageNumbers {
		t.Error("pageNumbers option lost")
	}
	if theme.vars["footerText"] != "Dummy" {
		t.Errorf("vars = %v", theme.vars)
	}
}

func TestListPDFThemesWarnsForInvalidPackageThemes(t *testing.T) {
	service := newPDFTestService(t)
	root := filepath.Join(service.root, ".notevault", "pdf-themes")

	// No manifest at all.
	writePDFThemePackage(t, service, "nomanifest", map[string]string{
		"document.html": validPDFThemeLayout,
	})
	// Manifest present, layout missing.
	writePDFThemePackage(t, service, "nolayout", map[string]string{
		pdfThemeManifestName: validPDFThemeManifest,
	})
	// Version 1 is reserved for flat themes.
	writePDFThemePackage(t, service, "oldversion", map[string]string{
		pdfThemeManifestName: strings.Replace(validPDFThemeManifest, `"version": 2`, `"version": 1`, 1),
		"document.html":      validPDFThemeLayout,
	})
	// Unknown manifest field.
	writePDFThemePackage(t, service, "unknownfield", map[string]string{
		pdfThemeManifestName: strings.Replace(validPDFThemeManifest, `"version": 2,`, `"version": 2, "script": "x",`, 1),
		"document.html":      validPDFThemeLayout,
	})
	// Layout escaping the theme directory.
	writePDFThemePackage(t, service, "traversal", map[string]string{
		pdfThemeManifestName: strings.Replace(validPDFThemeManifest, `"document.html"`, `"../../../etc/passwd.html"`, 1),
	})
	// Margins outside the accepted range.
	writePDFThemePackage(t, service, "badmargins", map[string]string{
		pdfThemeManifestName: strings.Replace(validPDFThemeManifest, `"top": 15`, `"top": 60`, 1),
		"document.html":      validPDFThemeLayout,
	})
	// Reserved identifier.
	writePDFThemePackage(t, service, "classic", map[string]string{
		pdfThemeManifestName: validPDFThemeManifest,
		"document.html":      validPDFThemeLayout,
	})
	// Invalid directory name.
	writePDFThemePackage(t, service, "Bad Name", map[string]string{
		pdfThemeManifestName: validPDFThemeManifest,
		"document.html":      validPDFThemeLayout,
	})
	// A flat theme already owns this identifier.
	writeTestFile(t, filepath.Join(root, "collision.css"), []byte(validPDFThemeStylesheet))
	writePDFThemePackage(t, service, "collision", map[string]string{
		pdfThemeManifestName: validPDFThemeManifest,
		"document.html":      validPDFThemeLayout,
	})
	// Symlinked directory.
	outside := t.TempDir()
	writeTestFile(t, filepath.Join(outside, pdfThemeManifestName), []byte(validPDFThemeManifest))
	writeTestFile(t, filepath.Join(outside, "document.html"), []byte(validPDFThemeLayout))
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}

	themes, warnings := service.ListPDFThemes()
	for _, id := range []string{
		"nomanifest", "nolayout", "oldversion", "unknownfield", "traversal",
		"badmargins", "Bad Name", "linked",
	} {
		if _, ok := findPDFTheme(themes, id); ok {
			t.Errorf("invalid theme %q was listed", id)
		}
		if !strings.Contains(strings.Join(warnings, "\n"), id) {
			t.Errorf("no warning mentions %q: %v", id, warnings)
		}
	}
	// The reserved identifier keeps the built-in theme.
	if theme, ok := findPDFTheme(themes, "classic"); !ok || !theme.Builtin {
		t.Error("the built-in classic theme was replaced by a package theme")
	}
	// The flat theme wins the collision and keeps working.
	if theme, ok := findPDFTheme(themes, "collision"); !ok || theme.layoutFile != "" {
		t.Error("the flat theme lost its identifier to a package directory")
	}
}

func TestListPDFThemesRejectsOversizedLayout(t *testing.T) {
	service := newPDFTestService(t)
	writePDFThemePackage(t, service, "huge", map[string]string{
		pdfThemeManifestName: validPDFThemeManifest,
		"document.html":      strings.Repeat("x", maxPDFLayoutBytes+1),
	})

	themes, warnings := service.ListPDFThemes()
	if _, ok := findPDFTheme(themes, "huge"); ok {
		t.Fatal("oversized layout was accepted")
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "256 Kio") {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}

func TestBuildNotePDFDocumentRendersPackageLayout(t *testing.T) {
	service := newPDFTestService(t)
	writePDFThemePackage(t, service, "dummy-package", map[string]string{
		pdfThemeManifestName: validPDFThemeManifest,
		"document.html": `<!doctype html><html><head><title>{{ get .Meta "title" }}</title>` +
			`<style>body{color:#111}</style></head><body>` +
			`<p class="tag">{{ index (get .Meta "tags") 0 }}</p>` +
			`<p class="path">{{ get .Meta "path" }}</p>` +
			`<footer>{{ get .Vars "footerText" }}</footer>` +
			`<main>{{ .Body }}</main></body></html>`,
	})
	note := createPDFTestNote(t, service, "Rapport", "# Rapport\n\nUn **paragraphe**.\n")
	note.Tags = []string{"projet"}
	note, err := service.SaveNote(note)
	if err != nil {
		t.Fatal(err)
	}

	document, err := service.BuildNotePDFDocument(note.RelativePath, "dummy-package", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	html := string(document.HTML)
	for _, expected := range []string{
		"<title>Rapport</title>",
		`<p class="tag">projet</p>`,
		"<footer>Dummy</footer>",
		"<strong>paragraphe</strong>",
		"body{color:#111}",
	} {
		if !strings.Contains(html, expected) {
			t.Errorf("rendered document misses %q\n%s", expected, html)
		}
	}
	// The layout owns the document: neither the built-in stylesheet nor the
	// NoteVault header survive.
	for _, forbidden := range []string{"document-title", "overflow-wrap:anywhere", "blocked-image"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("built-in document fragment %q leaked into the layout output", forbidden)
		}
	}
	if document.Margins.Top != 15 || !document.PageNumbers {
		t.Errorf("page options not taken from the manifest: %+v", document)
	}
}

func TestBuildNotePDFDocumentPackageLayoutEmbedsInternalDependencies(t *testing.T) {
	service := newPDFTestService(t)
	dir := writePDFThemePackage(t, service, "dummy-package", map[string]string{
		pdfThemeManifestName: validPDFThemeManifest,
		"document.html": `<!doctype html><html><head>` +
			`<style>@font-face{font-family:Geist;src:url({{ resolve "assets/font.woff2" }})}</style>` +
			`</head><body><img src="{{ resolve "assets/logo.png" }}" alt="logo">{{ .Body }}</body></html>`,
	})
	png, err := base64.StdEncoding.DecodeString(pdfTestPNG)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, "assets", "logo.png"), png)
	writeTestFile(t, filepath.Join(dir, "assets", "font.woff2"), []byte("wOF2\x00\x00\x00\x00"))
	note := createPDFTestNote(t, service, "Rapport", "Corps.\n")

	document, err := service.BuildNotePDFDocument(note.RelativePath, "dummy-package", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	html := string(document.HTML)
	if !strings.Contains(html, "data:image/png;base64,") {
		t.Error("internal image was not embedded")
	}
	// The MIME type comes from the extension: http.DetectContentType would
	// label a font as application/octet-stream.
	if !strings.Contains(html, "data:font/woff2;base64,") {
		t.Errorf("internal font was not embedded with its own MIME type\n%s", html)
	}
}

func TestPDFThemeResolverStaysInsideTheThemeDirectory(t *testing.T) {
	service := newPDFTestService(t)
	dir := writePDFThemePackage(t, service, "dummy-package", map[string]string{
		pdfThemeManifestName: validPDFThemeManifest,
		"document.html":      validPDFThemeLayout,
	})
	png, err := base64.StdEncoding.DecodeString(pdfTestPNG)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, "assets", "logo.png"), png)
	writeTestFile(t, filepath.Join(dir, "assets", "fake.png"), []byte("not a png"))
	writeTestFile(t, filepath.Join(dir, "assets", "tool.exe"), []byte("MZ"))
	writeTestFile(t, filepath.Join(service.root, "notes", "secret.md"), []byte("secret"))
	outside := filepath.Join(t.TempDir(), "outside.png")
	writeTestFile(t, outside, png)
	if err := os.Symlink(outside, filepath.Join(dir, "assets", "linked.png")); err != nil {
		t.Fatal(err)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	resolver := &pdfThemeResolver{root: root}

	if _, _, err := resolver.load("assets/logo.png"); err != nil {
		t.Fatalf("legitimate asset rejected: %v", err)
	}
	for _, rejected := range []string{
		"../../../notes/secret.md",
		"/etc/passwd",
		"https://example.test/pixel.png",
		"file:///etc/passwd",
		"stdin://",
		"assets/tool.exe",
		"assets/fake.png",
		"assets/linked.png",
		"assets",
	} {
		if _, _, err := resolver.load(rejected); err == nil {
			t.Errorf("resolver accepted %q", rejected)
		}
	}
}

func TestPDFThemeResolverEnforcesTotalBudget(t *testing.T) {
	service := newPDFTestService(t)
	dir := writePDFThemePackage(t, service, "dummy-package", map[string]string{
		pdfThemeManifestName: validPDFThemeManifest,
		"document.html":      validPDFThemeLayout,
	})
	writeTestFile(t, filepath.Join(dir, "assets", "big.css"), []byte(strings.Repeat("a", 4*1024*1024)))

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	resolver := &pdfThemeResolver{root: root}

	var lastErr error
	for range 10 {
		if _, _, lastErr = resolver.load("assets/big.css"); lastErr != nil {
			break
		}
	}
	if lastErr == nil {
		t.Fatal("the cumulative asset budget was never enforced")
	}
	if resolver.spent > maxPDFThemeAssetBudget {
		t.Fatalf("spent %d bytes, over the %d budget", resolver.spent, maxPDFThemeAssetBudget)
	}
}

func TestPDFThemeResolverRejectsOversizedAsset(t *testing.T) {
	service := newPDFTestService(t)
	dir := writePDFThemePackage(t, service, "dummy-package", map[string]string{
		pdfThemeManifestName: validPDFThemeManifest,
		"document.html":      validPDFThemeLayout,
	})
	writeTestFile(t, filepath.Join(dir, "assets", "big.css"), []byte(strings.Repeat("a", maxPDFThemeAssetBytes+1)))

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	resolver := &pdfThemeResolver{root: root}
	if _, _, err := resolver.load("assets/big.css"); err == nil {
		t.Fatal("oversized asset was accepted")
	}
}

func TestSanitizePDFLayoutOutputStripsActiveContentAndForcesCSP(t *testing.T) {
	raw := `<!doctype html><html><head>` +
		`<meta http-equiv="Content-Security-Policy" content="default-src *">` +
		`<base href="https://example.test/">` +
		`<link rel="stylesheet" href="https://example.test/theme.css">` +
		`<script>document.title='x'</script>` +
		`</head><body onload="steal()">` +
		`<img src="https://example.test/pixel.png" alt="remote">` +
		`<img src="data:image/png;base64,AAAA" alt="local">` +
		`<iframe src="data:text/html,hi"></iframe>` +
		`<a href="#anchor">ancre</a>` +
		`<a href="javascript:alert(1)">js</a>` +
		`<p>Texte</p></body></html>`

	sanitized, err := sanitizePDFLayoutOutput([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	output := string(sanitized)
	for _, forbidden := range []string{
		"<script", "<iframe", "<base", "<link", "onload",
		"https://example.test", "javascript:", "default-src *",
	} {
		if strings.Contains(output, forbidden) {
			t.Errorf("sanitized output still contains %q\n%s", forbidden, output)
		}
	}
	for _, expected := range []string{
		`src="data:image/png;base64,AAAA"`,
		`href="#anchor"`,
		"<p>Texte</p>",
		`alt="remote"`,
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("sanitized output lost %q\n%s", expected, output)
		}
	}
	if count := strings.Count(output, "Content-Security-Policy"); count != 1 {
		t.Errorf("CSP appears %d times, want exactly 1\n%s", count, output)
	}
	if !strings.Contains(output, "<head><meta http-equiv=\"Content-Security-Policy\"") {
		t.Errorf("the CSP is not the first child of <head>\n%s", output)
	}
	if !strings.Contains(output, "font-src data:") {
		t.Errorf("the layout CSP does not allow embedded fonts\n%s", output)
	}
}

func TestSanitizePDFLayoutOutputRequiresAHead(t *testing.T) {
	// html.Parse always synthesises <head>, so a document can only lack one
	// when the input is not a document at all.
	if _, err := sanitizePDFLayoutOutput([]byte("<p>fragment</p>")); err != nil {
		t.Fatalf("a fragment should still be sanitized into a document: %v", err)
	}
}

// pdfLayoutGoldenFuncs locks the template functions exposed to a theme layout.
// A sprig or amatl upgrade that adds a function fails here until it has been
// reviewed — the FuncMap is the whole attack surface of the template engine.
var pdfLayoutGoldenFuncs = []string{
	"abbrev", "abbrevboth", "add", "add1", "add1f", "addf", "adler32sum",
	"ago", "all", "any", "append", "atoi", "b32dec", "b32enc", "b64dec",
	"b64enc", "base", "bcrypt", "biggest", "buildCustomCert", "camelcase",
	"cat", "ceil", "chunk", "clean", "coalesce", "compact", "concat",
	"contains", "date", "dateInZone", "dateModify", "date_in_zone",
	"date_modify", "decryptAES", "deepCopy", "deepEqual", "default",
	"derivePassword", "dict", "dig", "dir", "div", "divf", "duration",
	"durationRound", "empty", "encryptAES", "ext", "fail", "first", "float64",
	"floor", "fromJson", "genCA", "genCAWithKey", "genPrivateKey",
	"genSelfSignedCert", "genSelfSignedCertWithKey", "genSignedCert",
	"genSignedCertWithKey", "get", "has", "hasKey", "hasPrefix", "hasSuffix",
	"hello", "htmlAddAttr", "htmlDate", "htmlDateInZone", "htmlQueryAll",
	"htmlQueryFirst", "htmlRemove", "htmlSplit", "htmlTextContent", "htpasswd",
	"indent", "initial", "initials", "int", "int64", "isAbs", "join",
	"kebabcase", "keys", "kindIs", "kindOf", "last", "list", "lower", "max",
	"maxf", "merge", "mergeOverwrite", "min", "minf", "mod", "mul", "mulf",
	"mustAppend", "mustChunk", "mustCompact", "mustDateModify", "mustDeepCopy",
	"mustFirst", "mustFromJson", "mustHas", "mustInitial", "mustLast",
	"mustMerge", "mustMergeOverwrite", "mustPrepend", "mustPush",
	"mustRegexFind", "mustRegexFindAll", "mustRegexMatch",
	"mustRegexReplaceAll", "mustRegexReplaceAllLiteral", "mustRegexSplit",
	"mustRest", "mustReverse", "mustSlice", "mustToDate", "mustToJson",
	"mustToPrettyJson", "mustToRawJson", "mustUniq", "mustWithout",
	"must_date_modify", "nindent", "nospace", "now", "omit", "osBase",
	"osClean", "osDir", "osExt", "osIsAbs", "pick", "pluck", "plural",
	"prepend", "push", "quote", "randAlpha", "randAlphaNum", "randAscii",
	"randBytes", "randInt", "randNumeric", "regexFind", "regexFindAll",
	"regexMatch", "regexQuoteMeta", "regexReplaceAll",
	"regexReplaceAllLiteral", "regexSplit", "repeat", "replace", "resolve",
	"rest", "reverse", "round", "semver", "semverCompare", "seq", "set",
	"sha1sum", "sha256sum", "sha512sum", "shuffle", "slice", "snakecase",
	"sortAlpha", "split", "splitList", "splitn", "squote", "sub", "subf",
	"substr", "swapcase", "ternary", "title", "toDate", "toDecimal", "toJson",
	"toPrettyJson", "toRawJson", "toString", "toStrings", "trim", "trimAll",
	"trimPrefix", "trimSuffix", "trimall", "trunc", "tuple", "typeIs",
	"typeIsLike", "typeOf", "uniq", "unixEpoch", "unset", "until", "untilStep",
	"untitle", "upper", "urlJoin", "urlParse", "uuidv4", "values", "without",
	"wrap", "wrapWith",
}

func TestPDFLayoutFuncsExposeOnlyReviewedFunctions(t *testing.T) {
	funcs := pdfLayoutFuncs(&pdfThemeResolver{})

	for _, forbidden := range pdfLayoutForbiddenFuncs {
		if _, present := funcs[forbidden]; present {
			t.Errorf("dangerous function %q is exposed to theme layouts", forbidden)
		}
	}

	golden := make(map[string]struct{}, len(pdfLayoutGoldenFuncs))
	for _, name := range pdfLayoutGoldenFuncs {
		golden[name] = struct{}{}
	}
	var unexpected []string
	for name := range funcs {
		if _, known := golden[name]; !known {
			unexpected = append(unexpected, name)
		}
	}
	sort.Strings(unexpected)
	if len(unexpected) > 0 {
		t.Fatalf("unreviewed template functions exposed to theme layouts: %v\n"+
			"review each one, then add it to pdfLayoutGoldenFuncs or to pdfLayoutForbiddenFuncs",
			unexpected)
	}
}

func TestRenderPDFLayoutRejectsOversizedOutput(t *testing.T) {
	service := newPDFTestService(t)
	writePDFThemePackage(t, service, "bomb", map[string]string{
		pdfThemeManifestName: validPDFThemeManifest,
		"document.html": `<!doctype html><html><head><title>x</title></head><body>` +
			`{{ range until 100000 }}{{ repeat 1000 "x" }}{{ end }}</body></html>`,
	})
	note := createPDFTestNote(t, service, "Bombe", "Corps.\n")

	if _, err := service.BuildNotePDFDocument(note.RelativePath, "bomb", false, nil); err == nil {
		t.Fatal("an unbounded layout output was accepted")
	}
}

func TestRenderPDFLayoutReportsTemplateErrors(t *testing.T) {
	service := newPDFTestService(t)
	writePDFThemePackage(t, service, "broken", map[string]string{
		pdfThemeManifestName: validPDFThemeManifest,
		"document.html":      `<html><body>{{ .Nope.Missing }}</body></html>`,
	})
	note := createPDFTestNote(t, service, "Cassé", "Corps.\n")

	_, err := service.BuildNotePDFDocument(note.RelativePath, "broken", false, nil)
	if err == nil {
		t.Fatal("a broken layout was accepted")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("the error does not name the theme: %v", err)
	}
}
