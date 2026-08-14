package vault

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"crypto/sha256"
)

// Diagrammes Mermaid dans l'export PDF.
//
// Le document imprimé porte une CSP `default-src 'none'` : aucun script n'y
// tourne, Mermaid ne peut donc pas être exécuté ici. Le frontend rend les
// diagrammes puis les transmet, indexés par l'empreinte de leur code source ;
// ce module valide les SVG reçus et les transforme en URI `data:`.
//
// Les SVG sont incorporés via `<img>` et non en ligne : dans un contexte
// `<img>` aucun script ni gestionnaire `on*` du SVG ne s'exécute, quelle que
// soit la CSP. Le contenu d'une note reste une entrée non fiable, mais il n'y
// a rien à assainir. La validation ci-dessous ne sert donc pas à contenir du
// code hostile : elle évite d'incorporer une image que le moteur de rendu
// refuserait d'afficher, et borne la taille du document.

const (
	mermaidLanguage      = "mermaid"
	mermaidSVGNamespace  = "http://www.w3.org/2000/svg"
	maxMermaidDiagrams   = 40
	maxMermaidSVGBytes   = 1 * 1024 * 1024
	maxMermaidTotalBytes = 8 * 1024 * 1024
)

var mermaidHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// normalizeMermaidCode aligns line endings with the frontend contract
// (frontend/src/lib/mermaid-blocks.ts): both sides hash the same bytes.
func normalizeMermaidCode(code string) string {
	code = strings.ReplaceAll(code, "\r\n", "\n")
	return strings.ReplaceAll(code, "\r", "\n")
}

// mermaidCodeHash mirrors sha256Hex(normalizeMermaidCode(code)) in the
// frontend. The trailing newline of a fenced block is dropped so that the
// goldmark segments and the mdast `code.value` describe the same text.
func mermaidCodeHash(code string) string {
	normalized := strings.TrimSuffix(normalizeMermaidCode(code), "\n")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// prepareMermaidDiagrams validates pre-rendered SVGs and converts them into
// data URIs keyed by code hash. Anything invalid or out of budget is dropped:
// the corresponding block simply falls back to its code rendering.
func prepareMermaidDiagrams(raw map[string]string) map[string]string {
	if len(raw) == 0 || len(raw) > maxMermaidDiagrams {
		return nil
	}
	hashes := make([]string, 0, len(raw))
	for hash := range raw {
		hashes = append(hashes, hash)
	}
	// Ordre déterministe : le budget cumulé ne doit pas dépendre de l'ordre
	// d'itération d'une map.
	sort.Strings(hashes)

	prepared := make(map[string]string, len(hashes))
	total := 0
	for _, hash := range hashes {
		svg := raw[hash]
		if !mermaidHashPattern.MatchString(hash) {
			continue
		}
		if len(svg) == 0 || len(svg) > maxMermaidSVGBytes {
			continue
		}
		if total+len(svg) > maxMermaidTotalBytes {
			break
		}
		if err := validateSVGDocument(svg); err != nil {
			continue
		}
		total += len(svg)
		prepared[hash] = "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
	}
	if len(prepared) == 0 {
		return nil
	}
	return prepared
}

// validateSVGDocument checks that the payload is well-formed XML whose root is
// an SVG element in the SVG namespace — exactly what a browser requires to
// display it through an `<img>`.
func validateSVGDocument(svg string) error {
	decoder := xml.NewDecoder(strings.NewReader(svg))
	decoder.Entity = xml.HTMLEntity
	seenRoot := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("SVG mal formé : %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || seenRoot {
			continue
		}
		if start.Name.Local != "svg" || start.Name.Space != mermaidSVGNamespace {
			return errors.New("racine SVG attendue")
		}
		seenRoot = true
	}
	if !seenRoot {
		return errors.New("document SVG vide")
	}
	return nil
}
