package vault

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ExportNotes écrit les notes (et leurs assets référencés) dans une archive
// zip unique. paths peut référencer des chemins relatifs (<root>/notes/...)
// ou des titres de notes : si le chemin n'existe pas, on cherche une note
// dont le titre correspond exactement (fallback pratique pour le menu
// contextuel qui ne dispose pas toujours du chemin).
//
// L'arborescence dans le zip reflète les chemins relatifs au coffre : les
// notes apparaissent dans notes/... et les images dans assets/... .
// Chaque note est préfixée par un en-tête YAML minimal (title + updated) si
// le frontmatter est manquant.
func (s *Service) ExportNotes(paths []string, destZip string) error {
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	if destZip == "" {
		return fmt.Errorf("chemin de destination vide")
	}
	if len(paths) == 0 {
		return fmt.Errorf("aucune note à exporter")
	}
	resolved, err := s.resolveExportPaths(paths)
	if err != nil {
		return err
	}
	if len(resolved) == 0 {
		return fmt.Errorf("aucune note trouvée pour les chemins fournis")
	}
	if err := os.MkdirAll(filepath.Dir(destZip), 0o755); err != nil {
		return fmt.Errorf("préparer le dossier de destination : %w", err)
	}
	out, err := os.Create(destZip)
	if err != nil {
		return fmt.Errorf("créer l'archive : %w", err)
	}
	success := false
	defer func() {
		if !success {
			_ = os.Remove(destZip)
		}
	}()
	zw := zip.NewWriter(out)
	added := make(map[string]struct{})
	for _, relPath := range resolved {
		raw, err := s.readPayload(relPath)
		if err != nil {
			_ = zw.Close()
			_ = out.Close()
			_ = os.Remove(destZip)
			return fmt.Errorf("lire %s : %w", relPath, err)
		}
		if err := writeZipEntry(zw, relPath, raw); err != nil {
			_ = zw.Close()
			_ = out.Close()
			_ = os.Remove(destZip)
			return err
		}
		added[relPath] = struct{}{}
		// Assets référencés dans le contenu Markdown.
		for _, asset := range referencedAssets(string(raw)) {
			// Le contenu Markdown est non fiable : un chemin forgé en
			// `assets/../../etc/...` passerait le préfixe sans ce filet.
			if _, err := normalizeAssetPath(asset); err != nil {
				continue
			}
			assetAbs := filepath.Join(s.root, filepath.FromSlash(asset))
			if _, err := os.Stat(assetAbs); err != nil {
				continue
			}
			if _, ok := added[asset]; ok {
				continue
			}
			data, err := os.ReadFile(assetAbs)
			if err != nil {
				continue
			}
			if err := writeZipEntry(zw, asset, data); err != nil {
				_ = zw.Close()
				_ = out.Close()
				_ = os.Remove(destZip)
				return err
			}
			added[asset] = struct{}{}
		}
	}
	if err := zw.Close(); err != nil {
		_ = out.Close()
		return fmt.Errorf("finaliser l'archive : %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("fermer l'archive : %w", err)
	}
	success = true
	return nil
}

// resolveExportPaths convertit une liste de chemins ou titres en une liste
// triée et dédupliquée de chemins relatifs. Les entrées invalides sont
// remontées en erreur pour éviter une archive silencieusement tronquée.
func (s *Service) resolveExportPaths(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	titleIndex := s.buildTitleIndex()
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		candidate := ""
		// Cas 1 : chemin relatif existant.
		if strings.HasPrefix(p, "notes/") {
			abs, pathErr := s.absoluteNotePath(p)
			if pathErr == nil {
				if _, statErr := os.Stat(abs); statErr == nil {
					candidate = filepath.ToSlash(filepath.Clean(filepath.FromSlash(p)))
				}
			}
		}
		// Cas 2 : titre exact (lookup préchargé).
		if candidate == "" {
			if rel, ok := titleIndex[p]; ok {
				candidate = rel
			}
		}
		if candidate == "" {
			return nil, fmt.Errorf("note introuvable : %q", p)
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	sort.Strings(out)
	return out, nil
}

// buildTitleIndex retourne un map title -> relativePath pour tous les
// fichiers présents dans notes/. Lecture directe sur disque : on n'utilise
// pas l'index mémoire pour rester cohérent avec l'état réel du coffre
// (un export doit toujours refléter les fichiers, pas un cache).
func (s *Service) buildTitleIndex() map[string]string {
	out := make(map[string]string)
	notesRoot := filepath.Join(s.root, "notes")
	_ = filepath.WalkDir(notesRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}
		rel, relErr := filepath.Rel(s.root, path)
		if relErr != nil {
			return nil
		}
		raw, err := s.readPayload(filepath.ToSlash(rel))
		if err != nil {
			return nil
		}
		note := parse(string(raw))
		title := strings.TrimSpace(note.Title)
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		out[title] = filepath.ToSlash(rel)
		return nil
	})
	return out
}

var mdImageRegex = regexp.MustCompile(`!\[[^\]]*\]\(([^)]+)\)`)

// referencedAssets retourne les chemins relatifs d'assets référencés dans
// un contenu Markdown. Ignore les URLs absolues (http, https, data, file).
func referencedAssets(md string) []string {
	matches := mdImageRegex.FindAllStringSubmatch(md, -1)
	out := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, m := range matches {
		raw := strings.TrimSpace(m[1])
		if raw == "" {
			continue
		}
		if strings.Contains(raw, "://") || strings.HasPrefix(raw, "data:") {
			continue
		}
		// Récupère le chemin avant tout titre Markdown (`![]()`).
		if idx := strings.Index(raw, " "); idx > 0 {
			raw = raw[:idx]
		}
		if !strings.HasPrefix(raw, "assets/") {
			continue
		}
		if _, ok := seen[raw]; ok {
			continue
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
	}
	return out
}

func writeZipEntry(zw *zip.Writer, relPath string, data []byte) error {
	relPath = filepath.ToSlash(relPath)
	w, err := zw.Create(relPath)
	if err != nil {
		return fmt.Errorf("créer l'entrée %s : %w", relPath, err)
	}
	if _, err := io.Copy(w, strings.NewReader(string(data))); err != nil {
		return fmt.Errorf("écrire %s : %w", relPath, err)
	}
	return nil
}
