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

	"github.com/kvitrvn/notevault/internal/domain"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
	_ "golang.org/x/image/webp"
)

const (
	maxPDFThemeBytes = 64 * 1024
	maxPDFAssetBytes = 20 * 1024 * 1024
	maxPDFImageSide  = 20_000
	maxPDFImagePixel = 100_000_000
)

var (
	pdfThemeIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	pdfColorPattern   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// PDFTheme is a declarative, versioned print theme. ID and Name are derived
// by NoteVault and are never read from user JSON.
type PDFTheme struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Builtin    bool               `json:"builtin"`
	Version    int                `json:"version"`
	Page       PDFPageTheme       `json:"page"`
	Typography PDFTypographyTheme `json:"typography"`
	Colors     PDFColorTheme      `json:"colors"`
	Options    PDFThemeOptions    `json:"options"`
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

func ensurePDFThemeDir(root string) error {
	if err := os.MkdirAll(filepath.Join(root, ".notevault", "pdf-themes"), 0o755); err != nil {
		return fmt.Errorf("créer le dossier des thèmes PDF : %w", err)
	}
	return nil
}

// ListPDFThemes returns the built-in theme, valid custom themes, and
// actionable warnings for rejected files.
func (s *Service) ListPDFThemes() ([]PDFTheme, []string) {
	themes := []PDFTheme{classicPDFTheme()}
	warnings := make([]string, 0)
	dir := filepath.Join(s.root, ".notevault", "pdf-themes")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return themes, warnings
	}
	if err != nil {
		return themes, []string{"Impossible de lire le dossier des thèmes PDF."}
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if id == "classic" {
			warnings = append(warnings, "classic.json : l’identifiant du thème intégré est réservé.")
			continue
		}
		theme, parseErr := loadPDFThemeFile(filepath.Join(dir, entry.Name()), id)
		if parseErr != nil {
			warnings = append(warnings, fmt.Sprintf("%s : %s", entry.Name(), parseErr))
			continue
		}
		themes = append(themes, theme)
	}
	sort.Slice(themes[1:], func(i, j int) bool {
		return themes[i+1].ID < themes[j+1].ID
	})
	return themes, warnings
}

func loadPDFThemeFile(path, id string) (PDFTheme, error) {
	if !pdfThemeIDPattern.MatchString(id) {
		return PDFTheme{}, errors.New("nom de fichier invalide")
	}
	file, err := os.Open(path)
	if err != nil {
		return PDFTheme{}, errors.New("fichier illisible")
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxPDFThemeBytes+1))
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
func (s *Service) BuildNotePDFDocument(relativePath, themeID string, plaintextConfirmed bool) (PDFDocument, error) {
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
	body, err := s.renderPDFMarkdown(note.Content)
	if err != nil {
		return PDFDocument{}, fmt.Errorf("rendre le Markdown : %w", err)
	}
	return PDFDocument{
		HTML:        buildPDFHTML(note, theme, body),
		Margins:     theme.Page.Margins,
		PageNumbers: theme.Options.PageNumbers,
	}, nil
}

func (s *Service) renderPDFMarkdown(markdown string) ([]byte, error) {
	var output bytes.Buffer
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(renderer.WithNodeRenderers(
			util.Prioritized(&pdfSafeNodeRenderer{service: s}, 500),
		)),
	)
	if err := md.Convert([]byte(markdown), &output); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

type pdfSafeNodeRenderer struct {
	service *Service
}

func (r *pdfSafeNodeRenderer) RegisterFuncs(register renderer.NodeRendererFuncRegisterer) {
	register.Register(ast.KindImage, r.renderImage)
	register.Register(ast.KindHTMLBlock, r.renderHTMLBlock)
	register.Register(ast.KindRawHTML, r.renderRawHTML)
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
	document.WriteString("}pre{padding:10pt;border-radius:4pt;background:")
	document.WriteString(theme.Colors.CodeBackground)
	document.WriteString(";white-space:pre-wrap;break-inside:avoid}code{font-size:.9em}table{width:100%;border-collapse:collapse;margin:1em 0}th,td{padding:5pt 7pt;border:1px solid ")
	document.WriteString(theme.Colors.Secondary)
	document.WriteString(";text-align:left;vertical-align:top}blockquote{margin-left:0;padding-left:12pt;border-left:3pt solid ")
	document.WriteString(theme.Colors.Accent)
	document.WriteString(";color:")
	document.WriteString(theme.Colors.Secondary)
	document.WriteString("}img{display:block;max-width:100%;max-height:22cm;margin:10pt auto;object-fit:contain}.blocked-image{color:")
	document.WriteString(theme.Colors.Secondary)
	document.WriteString(";font-style:italic}.document-title{margin:0 0 10pt}.title-page{display:grid;min-height:80vh;align-content:center;break-after:page}.metadata{color:")
	document.WriteString(theme.Colors.Secondary)
	document.WriteString(";font-size:.9em}.tags{margin-top:4pt}.task-list-item{list-style:none}input[type=checkbox]{margin-right:6pt}.raw-html{border-left:3pt solid ")
	document.WriteString(theme.Colors.Secondary)
	document.WriteString("}</style></head><body>")

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
