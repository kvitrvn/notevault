package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

var ErrInvalidVault = errors.New("ce dossier n’est pas un coffre NoteVault valide")

// CanonicalExistingDir resolves symlinks and verifies that path is a directory.
func CanonicalExistingDir(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("chemin vide")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("résoudre le chemin : %w", err)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("ouvrir le dossier : %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("vérifier le dossier : %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("le chemin sélectionné n’est pas un dossier")
	}
	return filepath.Clean(canonical), nil
}

// ValidateExistingVault checks an existing vault without creating or changing files.
func ValidateExistingVault(path string) (string, error) {
	canonical, err := CanonicalExistingDir(path)
	if err != nil {
		return "", err
	}
	for _, name := range []string{"notes", ".notevault"} {
		info, statErr := os.Lstat(filepath.Join(canonical, name))
		if statErr != nil || !info.IsDir() {
			return "", fmt.Errorf("%w : le dossier doit contenir notes/ et .notevault/ ; utilisez « Créer un coffre » pour un nouveau dossier", ErrInvalidVault)
		}
	}
	return canonical, nil
}

func ValidateVaultName(name string) error {
	if name != strings.TrimSpace(name) {
		return errors.New("le nom ne doit pas commencer ou finir par une espace")
	}
	length := len([]rune(name))
	if length < 1 || length > 80 {
		return errors.New("le nom doit contenir entre 1 et 80 caractères")
	}
	if name == "." || name == ".." {
		return errors.New("ce nom est réservé")
	}
	if strings.ContainsAny(name, `/\\`) {
		return errors.New("le nom ne doit pas contenir de séparateur de chemin")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return errors.New("le nom ne doit pas contenir de caractère de contrôle")
		}
	}
	return nil
}

// LegacyVaultUsed distinguishes a user vault from the empty tree created by old releases.
func LegacyVaultUsed(root string) (bool, error) {
	info, err := os.Stat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil || !info.IsDir() {
		return false, err
	}
	for _, dir := range []string{"notes", "assets", "templates", "themes"} {
		used, walkErr := directoryHasUserFile(filepath.Join(root, dir), dir == "notes")
		if walkErr != nil {
			return false, walkErr
		}
		if used {
			return true, nil
		}
	}
	metadata := filepath.Join(root, ".notevault")
	entries, err := os.ReadDir(metadata)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			children, readErr := os.ReadDir(filepath.Join(metadata, entry.Name()))
			if readErr == nil && len(children) > 0 {
				return true, nil
			}
			continue
		}
		if entry.Name() != "config.json" {
			return true, nil
		}
	}
	return false, nil
}

func directoryHasUserFile(root string, markdownOnly bool) (bool, error) {
	found := false
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, fs.ErrNotExist) {
			return fs.SkipDir
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !markdownOnly || strings.EqualFold(filepath.Ext(path), ".md") {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found, err
}
