package vault

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	htmlstd "html"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kvitrvn/notevault/internal/domain"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
	_ "golang.org/x/image/webp"
)

const (
	maxPDFThemeBytes      = 64 * 1024
	maxPDFStylesheetBytes = 64 * 1024
	maxPDFAssetBytes      = 20 * 1024 * 1024
	maxPDFImageSide       = 20_000
	maxPDFImagePixel      = 100_000_000
)

var (
	pdfThemeIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	pdfColorPattern   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// PDFTheme is a local print theme. Its rendering options remain declarative;
// an optional validated stylesheet is kept private to the vault service.
type PDFTheme struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Builtin    bool               `json:"builtin"`
	Version    int                `json:"version"`
	Page       PDFPageTheme       `json:"page"`
	Typography PDFTypographyTheme `json:"typography"`
	Colors     PDFColorTheme      `json:"colors"`
	Options    PDFThemeOptions    `json:"options"`
	stylesheet string
}

type PDFPageTheme struct {
	Size        string         `json:"size"`
	Orientation string         `json:"orientation"`
	Margins     PDFPageMargins `json:"margins"`
}

type PDFPageMargins struct {
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
}

type PDFTypographyTheme struct {
	Family       string  `json:"family"`
	MonoFamily   string  `json:"monoFamily"`
	BodySizePt   float64 `json:"bodySizePt"`
	LineHeight   float64 `json:"lineHeight"`
	HeadingScale float64 `json:"headingScale"`
}

type PDFColorTheme struct {
	Text           string `json:"text"`
	Secondary      string `json:"secondary"`
	Accent         string `json:"accent"`
	CodeBackground string `json:"codeBackground"`
}

type PDFThemeOptions struct {
	TitlePage   bool `json:"titlePage"`
	Metadata    bool `json:"metadata"`
	PageNumbers bool `json:"pageNumbers"`
}

type pdfThemeFile struct {
	Version    int                `json:"version"`
	Page       PDFPageTheme       `json:"page"`
	Typography PDFTypographyTheme `json:"typography"`
	Colors     PDFColorTheme      `json:"colors"`
	Options    PDFThemeOptions    `json:"options"`
}

type PDFExportOptionsInfo struct {
	Available         bool       `json:"available"`
	Browser           string     `json:"browser"`
	UnavailableReason string     `json:"unavailableReason"`
	Themes            []PDFTheme `json:"themes"`
	Warnings          []string   `json:"warnings"`
}

// PDFDocument contains only controlled HTML and validated rendering options.
// It is sent to the isolated worker by the parent process.
type PDFDocument struct {
	HTML        []byte
	Margins     PDFPageMargins
	PageNumbers bool
}

func classicPDFTheme() PDFTheme {
	return PDFTheme{
		ID:      "classic",
		Name:    "Classique",
		Builtin: true,
		Version: 1,
		Page: PDFPageTheme{
			Size:        "A4",
			Orientation: "portrait",
			Margins:     PDFPageMargins{Top: 20, Right: 18, Bottom: 20, Left: 18},
		},
		Typography: PDFTypographyTheme{
			Family:       "serif",
			MonoFamily:   "monospace",
			BodySizePt:   11,
			LineHeight:   1.5,
			HeadingScale: 1.25,
		},
		Colors: PDFColorTheme{
			Text:           "#202124",
			Secondary:      "#5f6368",
			Accent:         "#315c8c",
			CodeBackground: "#f3f4f6",
		},
		Options: PDFThemeOptions{TitlePage: false, Metadata: true, PageNumbers: true},
	}
}

func stylesheetPDFTheme(id string) PDFTheme {
	theme := classicPDFTheme()
	theme.ID = id
	theme.Name = id
	theme.Builtin = false
	return theme
}

func ensurePDFThemeDir(root string) error {
	if err := os.MkdirAll(filepath.Join(root, ".notevault", "pdf-themes"), 0o755); err != nil {
		return fmt.Errorf("créer le dossier des thèmes PDF : %w", err)
	}
	return nil
}

type pdfThemeFiles struct {
	json       string
	stylesheet string
}

// ListPDFThemes returns the built-in theme, valid local themes, and actionable
// warnings for rejected files. A CSS file can stand alone or complement JSON
// rendering options with the same identifier.
func (s *Service) ListPDFThemes() ([]PDFTheme, []string) {
	themes := []PDFTheme{classicPDFTheme()}
	reservedIDs := make(map[string]struct{}, len(themes))
	for _, theme := range themes {
		reservedIDs[theme.ID] = struct{}{}
	}
	warnings := make([]string, 0)
	dir := filepath.Join(s.root, ".notevault", "pdf-themes")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return themes, warnings
	}
	if err != nil {
		return themes, []string{"Impossible de lire le dossier des thèmes PDF."}
	}
	themeRoot, err := os.OpenRoot(dir)
	if err != nil {
		return themes, []string{"Impossible d’ouvrir le dossier des thèmes PDF."}
	}
	defer themeRoot.Close()

	filesByID := make(map[string]pdfThemeFiles)
	for _, entry := range entries {
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if entry.IsDir() || (extension != ".json" && extension != ".css") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			warnings = append(warnings, fmt.Sprintf("%s : les liens symboliques ne sont pas autorisés.", entry.Name()))
			continue
		}
		id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if _, reserved := reservedIDs[id]; reserved {
			warnings = append(warnings, fmt.Sprintf("%s : l’identifiant du thème intégré est réservé.", entry.Name()))
			continue
		}
		if !pdfThemeIDPattern.MatchString(id) {
			warnings = append(warnings, fmt.Sprintf("%s : nom de fichier invalide.", entry.Name()))
			continue
		}
		files := filesByID[id]
		if extension == ".json" {
			if files.json != "" {
				warnings = append(warnings, fmt.Sprintf("%s : plusieurs fichiers JSON utilisent cet identifiant.", entry.Name()))
				continue
			}
			files.json = entry.Name()
		} else {
			if files.stylesheet != "" {
				warnings = append(warnings, fmt.Sprintf("%s : plusieurs feuilles de style utilisent cet identifiant.", entry.Name()))
				continue
			}
			files.stylesheet = entry.Name()
		}
		filesByID[id] = files
	}

	ids := make([]string, 0, len(filesByID))
	for id := range filesByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		files := filesByID[id]
		theme := stylesheetPDFTheme(id)
		if files.json != "" {
			var parseErr error
			theme, parseErr = loadPDFThemeRootFile(themeRoot, files.json, id)
			if parseErr != nil {
				warnings = append(warnings, fmt.Sprintf("%s : %s", files.json, parseErr))
				continue
			}
		}
		if files.stylesheet != "" {
			stylesheet, parseErr := loadPDFThemeRootStylesheet(themeRoot, files.stylesheet)
			if parseErr != nil {
				warnings = append(warnings, fmt.Sprintf("%s : %s", files.stylesheet, parseErr))
				continue
			}
			theme.stylesheet = stylesheet
		}
		themes = append(themes, theme)
	}
	return themes, warnings
}

func loadPDFThemeFile(path, id string) (PDFTheme, error) {
	file, err := os.Open(path)
	if err != nil {
		return PDFTheme{}, errors.New("fichier illisible")
	}
	defer file.Close()
	return decodePDFTheme(file, id)
}

func loadPDFThemeRootFile(root *os.Root, name, id string) (PDFTheme, error) {
	file, err := root.Open(name)
	if err != nil {
		return PDFTheme{}, errors.New("fichier illisible")
	}
	defer file.Close()
	return decodePDFTheme(file, id)
}

func decodePDFTheme(reader io.Reader, id string) (PDFTheme, error) {
	if !pdfThemeIDPattern.MatchString(id) {
		return PDFTheme{}, errors.New("nom de fichier invalide")
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxPDFThemeBytes+1))
	if err != nil {
		return PDFTheme{}, errors.New("fichier illisible")
	}
	if len(raw) > maxPDFThemeBytes {
		return PDFTheme{}, errors.New("fichier supérieur à 64 Kio")
	}

	var fileTheme pdfThemeFile
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fileTheme); err != nil {
		return PDFTheme{}, errors.New("JSON invalide ou champ inconnu")
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return PDFTheme{}, errors.New("contenu supplémentaire après le JSON")
	}
	theme := PDFTheme{
		ID:         id,
		Name:       id,
		Version:    fileTheme.Version,
		Page:       fileTheme.Page,
		Typography: fileTheme.Typography,
		Colors:     fileTheme.Colors,
		Options:    fileTheme.Options,
	}
	if err := validatePDFTheme(theme); err != nil {
		return PDFTheme{}, err
	}
	return theme, nil
}

func loadPDFThemeStylesheet(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("fichier illisible")
	}
	defer file.Close()
	return decodePDFThemeStylesheet(file)
}

func loadPDFThemeRootStylesheet(root *os.Root, name string) (string, error) {
	file, err := root.Open(name)
	if err != nil {
		return "", errors.New("fichier illisible")
	}
	defer file.Close()
	return decodePDFThemeStylesheet(file)
}

func decodePDFThemeStylesheet(reader io.Reader) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxPDFStylesheetBytes+1))
	if err != nil {
		return "", errors.New("fichier illisible")
	}
	if len(raw) > maxPDFStylesheetBytes {
		return "", errors.New("fichier supérieur à 64 Kio")
	}
	if err := validatePDFThemeStylesheet(raw); err != nil {
		return "", err
	}
	return string(raw), nil
}

func validatePDFThemeStylesheet(raw []byte) error {
	if !utf8.Valid(raw) {
		return errors.New("CSS non UTF-8")
	}
	value := string(raw)
	for _, character := range value {
		if character == '\\' {
			return errors.New("les échappements CSS ne sont pas autorisés")
		}
		if character == '<' {
			return errors.New("le balisage HTML n’est pas autorisé")
		}
		if unicode.IsControl(character) && character != '\n' && character != '\r' && character != '\t' {
			return errors.New("caractère de contrôle interdit")
		}
	}

	compact, err := compactPDFStylesheet(value)
	if err != nil {
		return err
	}
	for _, forbidden := range []string{
		"url(",
		"@import",
		"@font-face",
		"@namespace",
		"@document",
		"expression(",
		"behavior:",
		"-moz-binding:",
		"http:",
		"https:",
		"file:",
		"ftp:",
		"data:",
	} {
		if strings.Contains(compact, forbidden) {
			return fmt.Errorf("construction CSS interdite : %s", forbidden)
		}
	}
	return nil
}

func compactPDFStylesheet(value string) (string, error) {
	var compact strings.Builder
	for index := 0; index < len(value); {
		if index+1 < len(value) && value[index:index+2] == "/*" {
			end := strings.Index(value[index+2:], "*/")
			if end < 0 {
				return "", errors.New("commentaire CSS non terminé")
			}
			index += end + 4
			continue
		}
		character, size := utf8.DecodeRuneInString(value[index:])
		index += size
		if unicode.IsSpace(character) {
			continue
		}
		compact.WriteRune(unicode.ToLower(character))
	}
	return compact.String(), nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("contenu JSON supplémentaire")
	}
	return err
}

func validatePDFTheme(theme PDFTheme) error {
	if theme.Version != 1 {
		return errors.New("version non supportée")
	}
	if theme.Page.Size != "A4" && theme.Page.Size != "Letter" {
		return errors.New("format de page invalide")
	}
	if theme.Page.Orientation != "portrait" && theme.Page.Orientation != "landscape" {
		return errors.New("orientation invalide")
	}
	for _, margin := range []float64{
		theme.Page.Margins.Top,
		theme.Page.Margins.Right,
		theme.Page.Margins.Bottom,
		theme.Page.Margins.Left,
	} {
		if margin < 5 || margin > 40 {
			return errors.New("les marges doivent être comprises entre 5 et 40 mm")
		}
	}
	if theme.Typography.Family != "serif" && theme.Typography.Family != "sans-serif" {
		return errors.New("famille de caractères invalide")
	}
	if theme.Typography.MonoFamily != "monospace" {
		return errors.New("police mono invalide")
	}
	if theme.Typography.BodySizePt < 9 || theme.Typography.BodySizePt > 18 {
		return errors.New("le corps doit être compris entre 9 et 18 pt")
	}
	if theme.Typography.LineHeight < 1.2 || theme.Typography.LineHeight > 2 {
		return errors.New("l’interligne doit être compris entre 1,2 et 2")
	}
	if theme.Typography.HeadingScale < 1 || theme.Typography.HeadingScale > 2 {
		return errors.New("l’échelle des titres doit être comprise entre 1 et 2")
	}
	for _, color := range []string{
		theme.Colors.Text,
		theme.Colors.Secondary,
		theme.Colors.Accent,
		theme.Colors.CodeBackground,
	} {
		if !pdfColorPattern.MatchString(color) {
			return errors.New("les couleurs doivent utiliser six chiffres hexadécimaux")
		}
	}
	return nil
}

func (s *Service) pdfTheme(id string) (PDFTheme, error) {
	if id == "" {
		id = "classic"
	}
	themes, _ := s.ListPDFThemes()
	for _, theme := range themes {
		if theme.ID == id {
			return theme, nil
		}
	}
	return PDFTheme{}, fmt.Errorf("thème PDF introuvable : %q", id)
}

// BuildNotePDFDocument reads a note from the unlocked vault and builds a
// self-contained HTML document. No remote or file URL is emitted.
//
// diagrams holds Mermaid diagrams already rendered to SVG by the frontend,
// keyed by the hash of their source code (see pdf_mermaid.go). It may be nil:
// the matching blocks then keep their code rendering.
func (s *Service) BuildNotePDFDocument(
	relativePath, themeID string,
	plaintextConfirmed bool,
	diagrams map[string]string,
) (PDFDocument, error) {
	if err := s.requireUnlocked(); err != nil {
		return PDFDocument{}, err
	}
	if s.VaultStatus().EncryptionEnabled && !plaintextConfirmed {
		return PDFDocument{}, errors.New("confirmez que le PDF contiendra la note en clair")
	}
	path, err := s.absoluteNotePath(relativePath)
	if err != nil {
		return PDFDocument{}, err
	}
	raw, err := s.readPayload(relativePath)
	if err != nil {
		return PDFDocument{}, fmt.Errorf("lire la note : %w", err)
	}
	note := parse(string(raw))
	note.RelativePath = relativePath
	if note.Title == "" {
		note.Title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if info, statErr := os.Stat(path); statErr == nil && note.UpdatedAt.IsZero() {
		note.UpdatedAt = info.ModTime().UTC()
	}
	theme, err := s.pdfTheme(themeID)
	if err != nil {
		return PDFDocument{}, err
	}
	body, err := s.renderPDFMarkdown(note.Content, diagrams)
	if err != nil {
		return PDFDocument{}, fmt.Errorf("rendre le Markdown : %w", err)
	}
	return PDFDocument{
		HTML:        buildPDFHTML(note, theme, body),
		Margins:     theme.Page.Margins,
		PageNumbers: theme.Options.PageNumbers,
	}, nil
}

func (s *Service) renderPDFMarkdown(markdown string, diagrams map[string]string) ([]byte, error) {
	var output bytes.Buffer
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(renderer.WithNodeRenderers(
			util.Prioritized(&pdfSafeNodeRenderer{
				service:  s,
				diagrams: prepareMermaidDiagrams(diagrams),
			}, 500),
		)),
	)
	if err := md.Convert([]byte(markdown), &output); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

type pdfSafeNodeRenderer struct {
	service *Service
	// Diagrammes Mermaid pré-rendus, indexés par empreinte du code source et
	// déjà validés sous forme d'URI `data:` (cf. pdf_mermaid.go).
	diagrams map[string]string
}

func (r *pdfSafeNodeRenderer) RegisterFuncs(register renderer.NodeRendererFuncRegisterer) {
	register.Register(ast.KindImage, r.renderImage)
	register.Register(ast.KindHTMLBlock, r.renderHTMLBlock)
	register.Register(ast.KindRawHTML, r.renderRawHTML)
	register.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
}

// renderFencedCodeBlock swaps a ```mermaid block for its pre-rendered SVG when
// one was supplied, and otherwise reproduces goldmark's own code rendering —
// the prioritized registration above replaces the built-in renderer entirely,
// so the fallback has to be written out here.
func (r *pdfSafeNodeRenderer) renderFencedCodeBlock(
	writer util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool,
) (ast.WalkStatus, error) {
	block := node.(*ast.FencedCodeBlock)
	language := block.Language(source)
	if strings.EqualFold(string(language), mermaidLanguage) {
		if uri, ok := r.diagrams[mermaidCodeHash(fencedCodeBlockText(block, source))]; ok {
			if entering {
				_, _ = writer.WriteString(`<img class="mermaid-diagram" src="`)
				_, _ = writer.WriteString(uri)
				_, _ = writer.WriteString(`" alt="Diagramme Mermaid">`)
			}
			return ast.WalkSkipChildren, nil
		}
	}
	if !entering {
		_, _ = writer.WriteString("</code></pre>\n")
		return ast.WalkContinue, nil
	}
	_, _ = writer.WriteString("<pre><code")
	if language != nil {
		_, _ = writer.WriteString(` class="language-`)
		_, _ = writer.Write(util.EscapeHTML(language))
		_, _ = writer.WriteString(`"`)
	}
	_ = writer.WriteByte('>')
	for index := 0; index < block.Lines().Len(); index++ {
		line := block.Lines().At(index)
		_, _ = writer.Write(util.EscapeHTML(line.Value(source)))
	}
	return ast.WalkContinue, nil
}

func fencedCodeBlockText(block *ast.FencedCodeBlock, source []byte) string {
	var text strings.Builder
	for index := 0; index < block.Lines().Len(); index++ {
		line := block.Lines().At(index)
		text.Write(line.Value(source))
	}
	return text.String()
}

func (r *pdfSafeNodeRenderer) renderImage(
	writer util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool,
) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	imageNode := node.(*ast.Image)
	alt := imageAltText(imageNode, source)
	dataURI, err := r.service.pdfAssetDataURI(string(imageNode.Destination))
	if err != nil {
		_, _ = writer.WriteString(`<span class="blocked-image">[Image`)
		if alt != "" {
			_, _ = writer.WriteString(` : `)
			_, _ = writer.Write(util.EscapeHTML([]byte(alt)))
		}
		_, _ = writer.WriteString(`]</span>`)
		return ast.WalkSkipChildren, nil
	}
	_, _ = writer.WriteString(`<img src="`)
	_, _ = writer.WriteString(dataURI)
	_, _ = writer.WriteString(`" alt="`)
	_, _ = writer.Write(util.EscapeHTML([]byte(alt)))
	_, _ = writer.WriteString(`">`)
	return ast.WalkSkipChildren, nil
}

func imageAltText(node ast.Node, source []byte) string {
	var text strings.Builder
	var visit func(ast.Node)
	visit = func(current ast.Node) {
		switch typed := current.(type) {
		case *ast.Text:
			text.Write(typed.Segment.Value(source))
		case *ast.String:
			text.Write(typed.Value)
		}
		for child := current.FirstChild(); child != nil; child = child.NextSibling() {
			visit(child)
		}
	}
	visit(node)
	return text.String()
}

func (r *pdfSafeNodeRenderer) renderHTMLBlock(
	writer util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool,
) (ast.WalkStatus, error) {
	block := node.(*ast.HTMLBlock)
	if entering {
		_, _ = writer.WriteString(`<pre class="raw-html"><code>`)
		for index := 0; index < block.Lines().Len(); index++ {
			line := block.Lines().At(index)
			_, _ = writer.Write(util.EscapeHTML(line.Value(source)))
		}
		return ast.WalkContinue, nil
	}
	if block.HasClosure() {
		_, _ = writer.Write(util.EscapeHTML(block.ClosureLine.Value(source)))
	}
	_, _ = writer.WriteString("</code></pre>\n")
	return ast.WalkContinue, nil
}

func (r *pdfSafeNodeRenderer) renderRawHTML(
	writer util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool,
) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	raw := node.(*ast.RawHTML)
	for index := 0; index < raw.Segments.Len(); index++ {
		segment := raw.Segments.At(index)
		_, _ = writer.Write(util.EscapeHTML(segment.Value(source)))
	}
	return ast.WalkSkipChildren, nil
}

func (s *Service) pdfAssetDataURI(relativePath string) (string, error) {
	assetPath, err := normalizeAssetPath(relativePath)
	if err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(assetPath))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
	default:
		return "", errors.New("format raster non supporté")
	}

	root, err := os.OpenRoot(filepath.Join(s.root, "assets"))
	if err != nil {
		return "", err
	}
	defer root.Close()
	file, err := root.Open(assetPath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() || info.Size() > maxPDFAssetBytes {
		return "", errors.New("asset invalide ou trop volumineux")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxPDFAssetBytes+1))
	if err != nil || len(data) > maxPDFAssetBytes {
		return "", errors.New("asset illisible ou trop volumineux")
	}
	contentType, err := validateRasterData(ext, data)
	if err != nil {
		return "", err
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func validateRasterData(ext string, data []byte) (string, error) {
	expected := map[string]string{
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".gif":  "image/gif",
		".webp": "image/webp",
	}[ext]
	detected := mime.TypeByExtension(ext)
	if separator := strings.IndexByte(detected, ';'); separator >= 0 {
		detected = detected[:separator]
	}
	if detected != "" && detected != expected {
		return "", errors.New("type d’image invalide")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return "", errors.New("contenu d’image invalide")
	}
	if config.Width > maxPDFImageSide || config.Height > maxPDFImageSide ||
		int64(config.Width)*int64(config.Height) > maxPDFImagePixel {
		return "", errors.New("dimensions d’image trop grandes")
	}
	if (format == "jpeg" && expected == "image/jpeg") || "image/"+format == expected {
		return expected, nil
	}
	return "", errors.New("extension et contenu d’image incompatibles")
}

func buildPDFHTML(note domain.Note, theme PDFTheme, body []byte) []byte {
	number := func(value float64) string {
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
	var document bytes.Buffer
	document.WriteString("<!doctype html><html lang=\"fr\"><head><meta charset=\"utf-8\">")
	document.WriteString(`<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'; font-src 'none'; connect-src 'none'; media-src 'none'; object-src 'none'; frame-src 'none'; base-uri 'none'; form-action 'none'">`)
	document.WriteString("<title>")
	document.WriteString(htmlstd.EscapeString(note.Title))
	document.WriteString("</title><style>")
	document.WriteString("@page{size:")
	document.WriteString(theme.Page.Size)
	document.WriteByte(' ')
	document.WriteString(theme.Page.Orientation)
	document.WriteString(";}*{box-sizing:border-box}html{color:")
	document.WriteString(theme.Colors.Text)
	document.WriteString(";background:#ffffff;font-family:")
	document.WriteString(theme.Typography.Family)
	document.WriteString("}body{margin:0;font-size:")
	document.WriteString(number(theme.Typography.BodySizePt))
	document.WriteString("pt;line-height:")
	document.WriteString(number(theme.Typography.LineHeight))
	document.WriteString(";overflow-wrap:anywhere}h1,h2,h3,h4,h5,h6{color:")
	document.WriteString(theme.Colors.Text)
	document.WriteString(";line-height:1.2;break-after:avoid}h1{font-size:")
	document.WriteString(number(theme.Typography.BodySizePt * theme.Typography.HeadingScale * 1.45))
	document.WriteString("pt}h2{font-size:")
	document.WriteString(number(theme.Typography.BodySizePt * theme.Typography.HeadingScale * 1.2))
	document.WriteString("pt}h3{font-size:")
	document.WriteString(number(theme.Typography.BodySizePt * theme.Typography.HeadingScale))
	document.WriteString("pt}a{color:")
	document.WriteString(theme.Colors.Accent)
	document.WriteString("}pre,code{font-family:")
	document.WriteString(theme.Typography.MonoFamily)
	document.WriteString("}pre{padding:7pt 9pt;border-radius:3pt;font-size:.88em;line-height:1.4;background:")
	document.WriteString(theme.Colors.CodeBackground)
	document.WriteString(";white-space:pre-wrap;break-inside:avoid}pre code{font-size:inherit}code{font-size:.9em}table{width:100%;border-collapse:collapse;margin:1em 0}th,td{padding:5pt 7pt;border:1px solid ")
	document.WriteString(theme.Colors.Secondary)
	document.WriteString(";text-align:left;vertical-align:top}blockquote{margin-left:0;padding-left:12pt;border-left:3pt solid ")
	document.WriteString(theme.Colors.Accent)
	document.WriteString(";color:")
	document.WriteString(theme.Colors.Secondary)
	document.WriteString("}img{display:block;max-width:100%;max-height:22cm;margin:10pt auto;object-fit:contain}img.mermaid-diagram{break-inside:avoid}.blocked-image{color:")
	document.WriteString(theme.Colors.Secondary)
	document.WriteString(";font-style:italic}.document-title{margin:0 0 10pt}.title-page{display:grid;min-height:80vh;align-content:center;break-after:page}.metadata{color:")
	document.WriteString(theme.Colors.Secondary)
	document.WriteString(";font-size:.9em}.tags{margin-top:4pt}.task-list-item{list-style:none}input[type=checkbox]{margin-right:6pt}.raw-html{border-left:3pt solid ")
	document.WriteString(theme.Colors.Secondary)
	document.WriteString("}")
	if theme.stylesheet != "" {
		document.WriteByte('\n')
		document.WriteString(theme.stylesheet)
	}
	document.WriteString("</style></head><body>")

	titleClass := "document-title"
	if theme.Options.TitlePage {
		document.WriteString(`<header class="title-page">`)
		titleClass += " title-page-heading"
	} else {
		document.WriteString("<header>")
	}
	document.WriteString(`<h1 class="`)
	document.WriteString(titleClass)
	document.WriteString(`">`)
	document.WriteString(htmlstd.EscapeString(note.Title))
	document.WriteString("</h1>")
	if theme.Options.Metadata {
		document.WriteString(`<div class="metadata">`)
		if !note.UpdatedAt.IsZero() {
			document.WriteString("<div>Mis à jour le ")
			document.WriteString(htmlstd.EscapeString(note.UpdatedAt.Local().Format("02/01/2006 à 15:04")))
			document.WriteString("</div>")
		}
		if len(note.Tags) > 0 {
			document.WriteString(`<div class="tags">Tags : `)
			for index, tag := range note.Tags {
				if index > 0 {
					document.WriteString(", ")
				}
				document.WriteString(htmlstd.EscapeString(tag))
			}
			document.WriteString("</div>")
		}
		document.WriteString("</div>")
	}
	document.WriteString("</header><main>")
	document.Write(body)
	document.WriteString("</main></body></html>")
	return document.Bytes()
}

// WritePDFAtomic verifies the signature and commits the output through a
// same-directory temporary file. No partial destination remains on failure.
func WritePDFAtomic(destination string, data []byte) error {
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return errors.New("la sortie du moteur n’est pas un PDF valide")
	}
	if destination == "" {
		return errors.New("destination PDF vide")
	}
	return writeAtomic(destination, data, 0o600)
}
