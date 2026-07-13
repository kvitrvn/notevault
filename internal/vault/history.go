package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kvitrvn/notevault/internal/domain"
)

// HistoryEntry représente une version archivée d'une note.
type HistoryEntry struct {
	ID        string    `json:"id"`        // timestamp RFC3339
	Timestamp time.Time `json:"timestamp"` // alias lisible
	Path      string    `json:"path"`      // chemin absolu du fichier d'historique
	Size      int64     `json:"size"`
	Preview   string    `json:"preview"` // 2 premières lignes du frontmatter
}

// historyDirFor retourne le dossier où stocker les versions d'une note.
func historyDirFor(root, relativePath string) string {
	return filepath.Join(root, ".notevault", "history", relativePath)
}

func (s *Service) snapshotHistory(relativePath string, maxVersions int) (string, error) {
	src := filepath.Join(s.root, filepath.FromSlash(relativePath))
	if _, err := os.Stat(src); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	raw, err := s.readPayload(relativePath)
	if err != nil {
		return "", err
	}
	dir := historyDirFor(s.root, relativePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	id := fmt.Sprintf("%d", nowUTC().UnixNano())
	dest := filepath.Join(dir, id+".md")
	destRel, err := filepath.Rel(s.root, dest)
	if err != nil {
		return "", err
	}
	if err := s.writePayload(filepath.ToSlash(destRel), raw, 0o600); err != nil {
		return "", err
	}
	if maxVersions > 0 {
		if err := rotateHistory(dir, maxVersions); err != nil {
			return id, err
		}
	}
	return id, nil
}

// rotateHistory supprime les versions les plus anciennes pour n'en
// garder que maxVersions.
func rotateHistory(dir string, maxVersions int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) <= maxVersions {
		return nil
	}
	infos := make([]os.DirEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		infos = append(infos, e)
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name() < infos[j].Name() })
	toDelete := len(infos) - maxVersions
	for i := 0; i < toDelete; i++ {
		_ = os.Remove(filepath.Join(dir, infos[i].Name()))
	}
	return nil
}

// ListHistory retourne les versions d'une note, de la plus récente à la plus ancienne.
func (s *Service) ListHistory(relativePath string) ([]HistoryEntry, error) {
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	if err := s.validateNoteRelPath(relativePath); err != nil {
		return nil, err
	}
	dir := historyDirFor(s.root, relativePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []HistoryEntry{}, nil
		}
		return nil, fmt.Errorf("lire l'historique : %w", err)
	}
	out := make([]HistoryEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		t, _ := time.Parse("", name) // la date est dérivée de l'ID
		if name != "" {
			if nano, err := strconv.ParseInt(name, 10, 64); err == nil {
				t = time.Unix(0, nano)
			}
		}
		info, _ := e.Info()
		var size int64
		if info != nil {
			size = info.Size()
		}
		preview := s.readHistoryPreview(filepath.Join(dir, e.Name()))
		out = append(out, HistoryEntry{
			ID:        name,
			Timestamp: t,
			Path:      filepath.ToSlash(filepath.Join(dir, e.Name())),
			Size:      size,
			Preview:   preview,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

func (s *Service) readHistoryPreview(path string) string {
	rel, err := filepath.Rel(s.root, path)
	if err != nil {
		return ""
	}
	raw, err := s.readPayload(filepath.ToSlash(rel))
	if err != nil {
		return ""
	}
	return historyPreview(string(raw))
}

func historyPreview(content string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, 2)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "title:") ||
			strings.HasPrefix(line, "created:") || strings.HasPrefix(line, "updated:") || strings.HasPrefix(line, "tags:") {
			continue
		}
		out = append(out, line)
		if len(out) == 2 {
			break
		}
	}
	return strings.Join(out, " / ")
}

// ReadHistoryVersion retourne le contenu brut d'une version d'historique.
func (s *Service) ReadHistoryVersion(relativePath, versionID string) (string, error) {
	if err := s.requireUnlocked(); err != nil {
		return "", err
	}
	if versionID == "" || strings.Trim(versionID, "0123456789") != "" {
		return "", fmt.Errorf("version invalide")
	}
	if err := s.validateNoteRelPath(relativePath); err != nil {
		return "", err
	}
	dir := historyDirFor(s.root, relativePath)
	path := filepath.Join(dir, versionID+".md")
	rel, err := filepath.Rel(s.root, path)
	if err != nil {
		return "", err
	}
	raw, err := s.readPayload(filepath.ToSlash(rel))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// RestoreFromHistory restaure une version d'historique comme version
// courante de la note.
func (s *Service) RestoreFromHistory(relativePath, versionID string) (domain.Note, error) {
	if err := s.validateNoteRelPath(relativePath); err != nil {
		return domain.Note{}, err
	}
	content, err := s.ReadHistoryVersion(relativePath, versionID)
	if err != nil {
		return domain.Note{}, err
	}
	// L'index sera mis à jour via SaveNote.
	note := parse(content)
	note.RelativePath = relativePath
	if note.Title == "" {
		note.Title = strings.TrimSuffix(filepath.Base(relativePath), filepath.Ext(relativePath))
	}
	return s.SaveNote(note)
}

// DiffHistory retourne un diff unifié entre deux versions (a -> b).
// Si l'une des versions est vide (absente), le diff est calculé contre
// une chaîne vide.
func (s *Service) DiffHistory(relativePath, aID, bID string) (string, error) {
	aRaw, _ := s.ReadHistoryVersion(relativePath, aID)
	bRaw, _ := s.ReadHistoryVersion(relativePath, bID)
	return unifiedDiff(aID, aRaw, bID, bRaw), nil
}
