package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/votre-compte/notevault/internal/domain"
)

type Service struct {
	root string
}

func NewDefaultService() (*Service, error) {
	root, err := defaultVaultPath()
	if err != nil {
		return nil, err
	}
	return NewService(root)
}

func NewService(root string) (*Service, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("résoudre la racine du coffre : %w", err)
	}
	for _, dir := range []string{"notes", "assets", "templates", ".notevault"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return nil, fmt.Errorf("créer %s : %w", dir, err)
		}
	}
	return &Service{root: root}, nil
}

func (s *Service) Root() string { return s.root }

func (s *Service) ListNotes() ([]domain.NoteSummary, error) {
	notesRoot := filepath.Join(s.root, "notes")
	notes := make([]domain.NoteSummary, 0)

	err := filepath.WalkDir(notesRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}
		note, err := s.readAbsolute(path)
		if err != nil {
			return err
		}
		notes = append(notes, domain.NoteSummary{
			RelativePath: note.RelativePath,
			Title:        note.Title,
			UpdatedAt:    note.UpdatedAt,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("lister les notes : %w", err)
	}

	sort.Slice(notes, func(i, j int) bool { return notes[i].UpdatedAt.After(notes[j].UpdatedAt) })
	return notes, nil
}

func (s *Service) OpenNote(relativePath string) (domain.Note, error) {
	path, err := s.absoluteNotePath(relativePath)
	if err != nil {
		return domain.Note{}, err
	}
	return s.readAbsolute(path)
}

func (s *Service) CreateNote(title string, templateKey string) (domain.Note, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Nouvelle note"
	}

	now := time.Now().UTC()
	filename := fmt.Sprintf("%s-%s.md", now.Format("20060102-150405"), slug(title))
	relativePath := filepath.ToSlash(filepath.Join("notes", "inbox", filename))
	content := template(templateKey)

	note := domain.Note{
		RelativePath: relativePath,
		Title:        title,
		Content:      content,
		CreatedAt:    now,
		UpdatedAt:    now,
		Tags:         []string{},
	}
	return s.SaveNote(note)
}

func (s *Service) SaveNote(note domain.Note) (domain.Note, error) {
	path, err := s.absoluteNotePath(note.RelativePath)
	if err != nil {
		return domain.Note{}, err
	}
	now := time.Now().UTC()
	if note.CreatedAt.IsZero() {
		note.CreatedAt = now
	}
	note.UpdatedAt = now
	note.Title = strings.TrimSpace(note.Title)
	if note.Title == "" {
		note.Title = "Sans titre"
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return domain.Note{}, fmt.Errorf("créer le dossier de la note : %w", err)
	}
	if err := os.WriteFile(path, []byte(serialize(note)), 0o644); err != nil {
		return domain.Note{}, fmt.Errorf("écrire la note : %w", err)
	}
	return note, nil
}

func (s *Service) absoluteNotePath(relativePath string) (string, error) {
	relativePath = filepath.Clean(filepath.FromSlash(relativePath))
	if relativePath == "." || filepath.IsAbs(relativePath) || strings.HasPrefix(relativePath, "..") {
		return "", errors.New("chemin de note invalide")
	}
	if filepath.Ext(relativePath) != ".md" {
		return "", errors.New("une note doit avoir l’extension .md")
	}
	if !strings.HasPrefix(filepath.ToSlash(relativePath), "notes/") {
		return "", errors.New("une note doit être rangée sous notes/")
	}
	return filepath.Join(s.root, relativePath), nil
}

func (s *Service) readAbsolute(path string) (domain.Note, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return domain.Note{}, fmt.Errorf("lire la note : %w", err)
	}
	relativePath, err := filepath.Rel(s.root, path)
	if err != nil {
		return domain.Note{}, err
	}
	note := parse(string(raw))
	note.RelativePath = filepath.ToSlash(relativePath)
	if note.Title == "" {
		note.Title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	info, err := os.Stat(path)
	if err == nil && note.UpdatedAt.IsZero() {
		note.UpdatedAt = info.ModTime().UTC()
	}
	return note, nil
}
