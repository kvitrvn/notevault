package vault

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	amatllayout "github.com/Bornholm/amatl/pkg/html/layout"
	amatlresolver "github.com/Bornholm/amatl/pkg/resolver"
	"github.com/kvitrvn/notevault/internal/domain"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Un thème PDF « package » est un dossier de `.notevault/pdf-themes/` contenant
// un manifeste `theme.json` et un layout HTML. Contrairement aux thèmes plats
// (`<id>.json` / `<id>.css`), le layout possède l'intégralité du document : ni
// la feuille de style intégrée ni l'en-tête titre/métadonnées de buildPDFHTML
// ne sont émis.
//
// Le contrôle de sécurité ne peut plus être une grammaire CSS — le layout écrit
// du CSS arbitraire dans son propre <style>. Il repose sur trois couches :
//
//  1. un resolver confiné au dossier du thème (aucun accès réseau ni fichier
//     hors de ce dossier, cf. pdfThemeResolver) ;
//  2. un assainissement du HTML produit par le template
//     (sanitizePDFLayoutOutput) ;
//  3. la CSP réinjectée de force en tête de <head>, jamais fournie par le thème.
const (
	pdfThemeManifestName = "theme.json"

	maxPDFLayoutBytes      = 256 * 1024
	maxPDFLayoutOutput     = 16 * 1024 * 1024
	maxPDFThemeAssetBytes  = 5 * 1024 * 1024
	maxPDFThemeAssetBudget = 24 * 1024 * 1024
	maxPDFThemeNameRunes   = 120

	pdfLayoutTimeout = 10 * time.Second
)

// pdfLayoutCSP est la politique appliquée aux documents produits par un layout.
// Elle élargit celle du chemin intégré (cf. buildPDFHTML) à `font-src data:`,
// les thèmes package pouvant embarquer leurs propres polices.
const pdfLayoutCSP = "default-src 'none'; img-src data:; font-src data:; " +
	"style-src 'unsafe-inline'; script-src 'none'; connect-src 'none'; " +
	"media-src 'none'; object-src 'none'; frame-src 'none'; base-uri 'none'; " +
	"form-action 'none'"

// pdfLayoutForbiddenFuncs retire du FuncMap sprig les fonctions capables de
// lire l'environnement du processus ou d'émettre une résolution DNS.
var pdfLayoutForbiddenFuncs = []string{"env", "expandenv", "getHostByName"}

// pdfThemeAssetTypes est l'allowlist des dépendances internes qu'un thème peut
// embarquer. Le type MIME est déduit de l'extension : celui que le layout
// passerait à `resolve` est ignoré, l'extension étant déjà contrainte ici.
var pdfThemeAssetTypes = map[string]string{
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".webp":  "image/webp",
	".svg":   "image/svg+xml",
	".woff2": "font/woff2",
	".woff":  "font/woff",
	".ttf":   "font/ttf",
	".otf":   "font/otf",
	".css":   "text/css",
	".html":  "text/html",
}

type pdfThemeManifest struct {
	Version int             `json:"version"`
	Name    string          `json:"name"`
	Layout  string          `json:"layout"`
	Page    PDFPageTheme    `json:"page"`
	Options PDFThemeOptions `json:"options"`
	Vars    map[string]any  `json:"vars"`
}

// loadPDFThemePackage lit le manifeste d'un thème package et vérifie que son
// layout existe. Le layout lui-même n'est ni lu ni exécuté ici : ListPDFThemes
// est appelé à chaque ouverture du dialogue d'export.
func loadPDFThemePackage(themeRoot *os.Root, id string) (PDFTheme, error) {
	root, err := themeRoot.OpenRoot(id)
	if err != nil {
		return PDFTheme{}, errors.New("dossier illisible")
	}
	defer root.Close()

	manifest, err := root.Open(pdfThemeManifestName)
	if err != nil {
		return PDFTheme{}, fmt.Errorf("%s manquant ou illisible", pdfThemeManifestName)
	}
	defer manifest.Close()
	theme, err := decodePDFThemeManifest(manifest, id)
	if err != nil {
		return PDFTheme{}, err
	}

	layout, err := root.Open(theme.layoutFile)
	if err != nil {
		return PDFTheme{}, fmt.Errorf("layout %q introuvable", theme.layoutFile)
	}
	defer layout.Close()
	info, err := layout.Stat()
	if err != nil || info.IsDir() {
		return PDFTheme{}, fmt.Errorf("layout %q invalide", theme.layoutFile)
	}
	if info.Size() > maxPDFLayoutBytes {
		return PDFTheme{}, fmt.Errorf("layout %q supérieur à 256 Kio", theme.layoutFile)
	}
	return theme, nil
}

func decodePDFThemeManifest(reader io.Reader, id string) (PDFTheme, error) {
	if !pdfThemeIDPattern.MatchString(id) {
		return PDFTheme{}, errors.New("nom de dossier invalide")
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxPDFThemeBytes+1))
	if err != nil {
		return PDFTheme{}, errors.New("fichier illisible")
	}
	if len(raw) > maxPDFThemeBytes {
		return PDFTheme{}, errors.New("fichier supérieur à 64 Kio")
	}

	var manifest pdfThemeManifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return PDFTheme{}, errors.New("JSON invalide ou champ inconnu")
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return PDFTheme{}, errors.New("contenu supplémentaire après le JSON")
	}
	if manifest.Version != 2 {
		return PDFTheme{}, errors.New("version non supportée (2 attendue)")
	}
	name, err := validatePDFThemeName(manifest.Name, id)
	if err != nil {
		return PDFTheme{}, err
	}
	layoutFile, err := validatePDFThemeLayoutPath(manifest.Layout)
	if err != nil {
		return PDFTheme{}, err
	}
	if err := validatePDFPage(manifest.Page); err != nil {
		return PDFTheme{}, err
	}

	// Typography et Colors n'existent pas en v2 — le layout possède le style —
	// mais restent renseignées par défaut pour que le type exposé au frontend
	// garde une forme cohérente.
	base := classicPDFTheme()
	return PDFTheme{
		ID:         id,
		Name:       name,
		Builtin:    false,
		Version:    manifest.Version,
		Page:       manifest.Page,
		Typography: base.Typography,
		Colors:     base.Colors,
		Options:    manifest.Options,
		layoutFile: layoutFile,
		vars:       manifest.Vars,
	}, nil
}

func validatePDFThemeName(name, id string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return id, nil
	}
	if utf8.RuneCountInString(name) > maxPDFThemeNameRunes {
		return "", errors.New("nom de thème trop long")
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return "", errors.New("nom de thème contenant un caractère de contrôle")
		}
	}
	return name, nil
}

func validatePDFThemeLayoutPath(value string) (string, error) {
	if value == "" {
		return "", errors.New("champ layout requis")
	}
	clean, err := normalizePDFThemeAssetPath(value)
	if err != nil {
		return "", errors.New("chemin de layout invalide")
	}
	switch strings.ToLower(filepath.Ext(clean)) {
	case ".html", ".htm":
	default:
		return "", errors.New("le layout doit être un fichier .html")
	}
	return clean, nil
}

// normalizePDFThemeAssetPath refuse tout ce qui pourrait viser hors du dossier
// du thème avant même l'ouverture. os.Root fournit la garantie effective ;
// cette étape sert à produire des messages lisibles (même approche que
// normalizeAssetPath / ResolveAsset).
func normalizePDFThemeAssetPath(value string) (string, error) {
	if value == "" {
		return "", errors.New("chemin vide")
	}
	clean := filepath.Clean(filepath.FromSlash(value))
	if !filepath.IsLocal(clean) {
		return "", errors.New("chemin hors du dossier du thème")
	}
	return clean, nil
}

// pdfThemeResolver implémente amatlresolver.Resolver en restant confiné au
// dossier du thème : aucun schéma d'URL n'est accepté, donc ni http(s), ni
// file, ni stdin, ni amatl — le resolver par défaut d'amatl n'est jamais
// utilisé.
type pdfThemeResolver struct {
	root  *os.Root
	spent int64
}

func (r *pdfThemeResolver) Resolve(_ context.Context, path amatlresolver.Path) (io.ReadCloser, error) {
	data, _, err := r.load(string(path))
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (r *pdfThemeResolver) load(rawPath string) ([]byte, string, error) {
	if amatlresolver.Path(rawPath).IsURL() {
		return nil, "", fmt.Errorf("ressource externe refusée : %q", rawPath)
	}
	clean, err := normalizePDFThemeAssetPath(rawPath)
	if err != nil {
		return nil, "", fmt.Errorf("ressource %q : %w", rawPath, err)
	}
	extension := strings.ToLower(filepath.Ext(clean))
	contentType, allowed := pdfThemeAssetTypes[extension]
	if !allowed {
		return nil, "", fmt.Errorf("extension non supportée : %q", extension)
	}

	file, err := r.root.Open(clean)
	if err != nil {
		return nil, "", fmt.Errorf("ressource introuvable : %q", rawPath)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return nil, "", fmt.Errorf("ressource invalide : %q", rawPath)
	}
	if info.Size() > maxPDFThemeAssetBytes {
		return nil, "", fmt.Errorf("ressource %q supérieure à 5 Mio", rawPath)
	}
	if r.spent+info.Size() > maxPDFThemeAssetBudget {
		return nil, "", errors.New("budget de ressources du thème dépassé")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxPDFThemeAssetBytes+1))
	if err != nil || int64(len(data)) > maxPDFThemeAssetBytes {
		return nil, "", fmt.Errorf("ressource illisible : %q", rawPath)
	}
	if r.spent+int64(len(data)) > maxPDFThemeAssetBudget {
		return nil, "", errors.New("budget de ressources du thème dépassé")
	}
	if err := validatePDFThemeAsset(extension, data); err != nil {
		return nil, "", fmt.Errorf("ressource %q : %w", rawPath, err)
	}
	r.spent += int64(len(data))
	return data, contentType, nil
}

// resolveFunc remplace la fonction `resolve` d'amatl : celle-ci déduit le type
// MIME via http.DetectContentType, qui étiquette les polices en
// application/octet-stream.
func (r *pdfThemeResolver) resolveFunc() func(string, ...string) (htmltemplate.URL, error) {
	return func(rawPath string, _ ...string) (htmltemplate.URL, error) {
		data, contentType, err := r.load(rawPath)
		if err != nil {
			return "", err
		}
		return htmltemplate.URL("data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)), nil
	}
}

func validatePDFThemeAsset(extension string, data []byte) error {
	switch extension {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		if _, err := validateRasterData(extension, data); err != nil {
			return err
		}
	case ".svg":
		if err := validateSVGDocument(string(data)); err != nil {
			return err
		}
	case ".css", ".html":
		if !utf8.Valid(data) {
			return errors.New("contenu non UTF-8")
		}
	case ".woff2", ".woff", ".ttf", ".otf":
		if !validFontSignature(extension, data) {
			return errors.New("signature de police invalide")
		}
	default:
		return errors.New("extension non supportée")
	}
	return nil
}

func validFontSignature(extension string, data []byte) bool {
	if len(data) < 4 {
		return false
	}
	switch extension {
	case ".woff2":
		return bytes.HasPrefix(data, []byte("wOF2"))
	case ".woff":
		return bytes.HasPrefix(data, []byte("wOFF"))
	case ".otf":
		return bytes.HasPrefix(data, []byte("OTTO"))
	case ".ttf":
		return bytes.HasPrefix(data, []byte{0x00, 0x01, 0x00, 0x00}) ||
			bytes.HasPrefix(data, []byte("true")) ||
			bytes.HasPrefix(data, []byte("ttcf"))
	}
	return false
}

// pdfLayoutFuncs part du FuncMap d'amatl (sprig + helpers htmlQuery*) et en
// retire les fonctions dangereuses. Un test golden verrouille la liste des noms
// exposés : une montée de version de sprig ou d'amatl qui ajoute une fonction
// échoue tant qu'elle n'a pas été relue.
func pdfLayoutFuncs(resolver *pdfThemeResolver) htmltemplate.FuncMap {
	funcs := amatllayout.DefaultFuncs(resolver)
	for _, name := range pdfLayoutForbiddenFuncs {
		delete(funcs, name)
	}
	funcs["resolve"] = resolver.resolveFunc()
	return funcs
}

// renderPDFLayout exécute le layout du thème puis assainit sa sortie.
//
// Le template est du contenu que l'utilisateur a lui-même déposé dans son
// vault — même niveau de confiance que ses notes. limitedWriter borne la taille
// produite et interrompt les boucles qui écrivent ; une boucle purement CPU
// sans écriture échappe à cette borne, d'où le chien de garde : il rend la main
// à l'utilisateur, mais la goroutine abandonnée peut lui survivre (DoS
// auto-infligé uniquement).
func (s *Service) renderPDFLayout(theme PDFTheme, note domain.Note, body []byte) ([]byte, error) {
	root, err := os.OpenRoot(filepath.Join(s.root, ".notevault", "pdf-themes", theme.ID))
	if err != nil {
		return nil, errors.New("dossier du thème PDF illisible")
	}
	defer root.Close()

	resolver := &pdfThemeResolver{root: root}
	vars := make(map[string]any, len(theme.vars)+2)
	for key, value := range theme.vars {
		vars[key] = value
	}
	vars["page"] = theme.Page
	vars["options"] = theme.Options
	meta := map[string]any{
		"title":     note.Title,
		"updatedAt": note.UpdatedAt,
		"tags":      note.Tags,
		"path":      note.RelativePath,
	}

	ctx, cancel := context.WithTimeout(context.Background(), pdfLayoutTimeout)
	defer cancel()

	type layoutResult struct {
		data []byte
		err  error
	}
	results := make(chan layoutResult, 1)
	go func() {
		writer := &limitedWriter{limit: maxPDFLayoutOutput}
		renderErr := amatllayout.Render(ctx, writer, body,
			amatllayout.WithURL(filepath.ToSlash(theme.layoutFile)),
			amatllayout.WithResolver(resolver),
			func(options *amatllayout.LayoutOptions) {
				options.Funcs = pdfLayoutFuncs(resolver)
			},
			amatllayout.WithVars(vars),
			amatllayout.WithMeta(meta),
		)
		results <- layoutResult{data: writer.Bytes(), err: renderErr}
	}()

	select {
	case result := <-results:
		if result.err != nil {
			return nil, fmt.Errorf("exécuter le layout du thème %q : %w", theme.ID, result.err)
		}
		return sanitizePDFLayoutOutput(result.data)
	case <-ctx.Done():
		return nil, fmt.Errorf("le layout du thème %q a dépassé le délai de %s", theme.ID, pdfLayoutTimeout)
	}
}

type limitedWriter struct {
	buffer bytes.Buffer
	limit  int
}

func (w *limitedWriter) Write(data []byte) (int, error) {
	if w.buffer.Len()+len(data) > w.limit {
		return 0, errors.New("document produit par le thème trop volumineux")
	}
	return w.buffer.Write(data)
}

func (w *limitedWriter) Bytes() []byte { return w.buffer.Bytes() }

// pdfLayoutDroppedElements sont supprimés avec leur sous-arbre. <link> et
// <base> partent aussi : la CSP les neutraliserait, mais un document sans eux
// est plus simple à raisonner.
var pdfLayoutDroppedElements = map[string]struct{}{
	"script": {}, "iframe": {}, "object": {}, "embed": {}, "applet": {},
	"base": {}, "link": {}, "form": {}, "template": {}, "frame": {},
	"frameset": {}, "portal": {},
}

// pdfLayoutURLAttributes ne conservent qu'une valeur `data:` ou un fragment.
var pdfLayoutURLAttributes = map[string]struct{}{
	"src": {}, "href": {}, "poster": {}, "data": {}, "action": {},
	"cite": {}, "longdesc": {}, "usemap": {}, "formaction": {}, "background": {},
}

// pdfLayoutDroppedAttributes sont retirés quelle que soit leur valeur.
var pdfLayoutDroppedAttributes = map[string]struct{}{
	"srcdoc": {}, "srcset": {}, "ping": {}, "xlink:href": {}, "http-equiv": {},
}

// sanitizePDFLayoutOutput est la seconde barrière : même si un layout produit
// du script ou une référence distante, rien ne survit au parcours ci-dessous,
// et la CSP est réinjectée en premier enfant de <head> pour que le thème ne
// puisse pas la remplacer par une politique plus permissive.
func sanitizePDFLayoutOutput(raw []byte) ([]byte, error) {
	document, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("analyser le document du thème : %w", err)
	}
	sanitizePDFLayoutNode(document)

	head := findPDFLayoutElement(document, atom.Head)
	if head == nil {
		return nil, errors.New("le layout du thème ne produit pas de <head>")
	}
	head.InsertBefore(&html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Meta,
		Data:     "meta",
		Attr: []html.Attribute{
			{Key: "http-equiv", Val: "Content-Security-Policy"},
			{Key: "content", Val: pdfLayoutCSP},
		},
	}, head.FirstChild)

	var output bytes.Buffer
	if err := html.Render(&output, document); err != nil {
		return nil, fmt.Errorf("sérialiser le document du thème : %w", err)
	}
	return output.Bytes(), nil
}

func sanitizePDFLayoutNode(node *html.Node) {
	var removals []*html.Node
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.CommentNode {
			removals = append(removals, child)
			continue
		}
		if child.Type == html.ElementNode {
			if _, dropped := pdfLayoutDroppedElements[child.Data]; dropped {
				removals = append(removals, child)
				continue
			}
			// Un <meta http-equiv> du thème pourrait affaiblir la CSP ou
			// déclencher un refresh : on ne garde que les autres.
			if child.DataAtom == atom.Meta && hasPDFLayoutAttribute(child, "http-equiv") {
				removals = append(removals, child)
				continue
			}
			child.Attr = sanitizePDFLayoutAttributes(child.Attr)
		}
		sanitizePDFLayoutNode(child)
	}
	for _, child := range removals {
		node.RemoveChild(child)
	}
}

func sanitizePDFLayoutAttributes(attributes []html.Attribute) []html.Attribute {
	kept := attributes[:0]
	for _, attribute := range attributes {
		key := strings.ToLower(attribute.Key)
		if attribute.Namespace != "" {
			key = strings.ToLower(attribute.Namespace) + ":" + key
		}
		if strings.HasPrefix(key, "on") {
			continue
		}
		if _, dropped := pdfLayoutDroppedAttributes[key]; dropped {
			continue
		}
		if _, isURL := pdfLayoutURLAttributes[key]; isURL && !allowedPDFLayoutURL(attribute.Val) {
			continue
		}
		kept = append(kept, attribute)
	}
	return kept
}

func allowedPDFLayoutURL(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return true
	}
	return strings.HasPrefix(strings.ToLower(trimmed), "data:")
}

func hasPDFLayoutAttribute(node *html.Node, name string) bool {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, name) {
			return true
		}
	}
	return false
}

func findPDFLayoutElement(node *html.Node, target atom.Atom) *html.Node {
	if node.Type == html.ElementNode && node.DataAtom == target {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findPDFLayoutElement(child, target); found != nil {
			return found
		}
	}
	return nil
}
