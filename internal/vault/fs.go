package vault

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// writeAtomic écrit des données dans path en passant par un fichier
// temporaire renommé à la fin. Évite la corruption si le processus est tué
// pendant l'écriture.
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("créer le dossier parent : %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("créer le fichier temporaire : %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("écrire dans le fichier temporaire : %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("synchroniser le fichier temporaire : %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return fmt.Errorf("ajuster les permissions : %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("fermer le fichier temporaire : %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("renommer le fichier : %w", err)
	}
	cleanup = false
	return nil
}

// TrashEntry décrit un fichier dans la corbeille, avec son chemin d'origine
// et la date de mise à la corbeille.
type TrashEntry struct {
	ID           string    `json:"id"`
	OriginalPath string    `json:"originalPath"`
	TrashPath    string    `json:"trashPath"`
	TrashedAt    time.Time `json:"trashedAt"`
	Size         int64     `json:"size"`
}

// softDelete déplace path dans <root>/.trash/<yyyy-mm-dd>/<basename>.
// Conserve le chemin d'origine dans un sidecar .meta pour la restauration.
func softDelete(root, path string) (TrashEntry, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return TrashEntry{}, fmt.Errorf("calculer le chemin relatif : %w", err)
	}
	rel = filepath.ToSlash(rel)
	now := time.Now().UTC()
	day := now.Format("2006-01-02")
	trashDir := filepath.Join(root, ".trash", day)
	if err := os.MkdirAll(trashDir, 0o755); err != nil {
		return TrashEntry{}, fmt.Errorf("créer le dossier de corbeille : %w", err)
	}
	filename := fmt.Sprintf("%s-%s", now.Format("20060102T150405.000"), filepath.Base(rel))
	dest := filepath.Join(trashDir, filename)
	if err := os.Rename(path, dest); err != nil {
		return TrashEntry{}, fmt.Errorf("déplacer vers la corbeille : %w", err)
	}
	id := filepath.ToSlash(filepath.Join(day, filename))
	meta := []byte("original: " + rel + "\ntrashed_at: " + now.Format(time.RFC3339) + "\n")
	metaPath := dest + ".meta"
	if err := os.WriteFile(metaPath, meta, 0o644); err != nil {
		// Non-bloquant : la restauration sera partielle mais la note est déjà isolée.
		_ = err
	}
	info, _ := os.Stat(dest)
	var size int64
	if info != nil {
		size = info.Size()
	}
	return TrashEntry{
		ID:           id,
		OriginalPath: rel,
		TrashPath:    filepath.ToSlash(dest),
		TrashedAt:    now,
		Size:         size,
	}, nil
}

// restoreFromTrash remet en place un fichier précédemment déplacé par softDelete.
// Renvoie le chemin relatif d'origine si connu.
func restoreFromTrash(root string, entry TrashEntry) (string, error) {
	metaPath := entry.TrashPath + ".meta"
	original := entry.OriginalPath
	if data, err := os.ReadFile(metaPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			key, value, found := strings.Cut(line, ":")
			if found && strings.TrimSpace(key) == "original" {
				original = strings.TrimSpace(value)
				break
			}
		}
	}
	dest := filepath.Join(root, filepath.FromSlash(original))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", fmt.Errorf("préparer le dossier de destination : %w", err)
	}
	if _, err := os.Stat(dest); err == nil {
		return "", fmt.Errorf("un fichier existe déjà à %s", original)
	}
	if err := os.Rename(entry.TrashPath, dest); err != nil {
		return "", fmt.Errorf("restaurer depuis la corbeille : %w", err)
	}
	_ = os.Remove(metaPath)
	return original, nil
}

// listTrash retourne toutes les entrées de la corbeille, triées de la plus
// récente à la plus ancienne.
func listTrash(root string) ([]TrashEntry, error) {
	trashRoot := filepath.Join(root, ".trash")
	entries := make([]TrashEntry, 0)
	err := filepath.WalkDir(trashRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".meta") {
			return nil
		}
		if strings.HasSuffix(path, ".purge_after") {
			return nil
		}
		rel, err := filepath.Rel(trashRoot, path)
		if err != nil {
			return nil
		}
		parts := strings.SplitN(filepath.ToSlash(rel), "/", 2)
		if len(parts) != 2 {
			return nil
		}
		day := parts[0]
		filename := parts[1]
		trashedAt, _ := time.Parse("20060102T150405.000", strings.SplitN(filename, "-", 2)[0])
		info, _ := d.Info()
		var size int64
		if info != nil {
			size = info.Size()
		}
		original := ""
		if data, err := os.ReadFile(path + ".meta"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				key, value, found := strings.Cut(line, ":")
				if !found {
					continue
				}
				switch strings.TrimSpace(key) {
				case "original":
					original = strings.TrimSpace(value)
				case "trashed_at":
					if t, perr := time.Parse(time.RFC3339, strings.TrimSpace(value)); perr == nil {
						trashedAt = t
					}
				}
			}
		}
		if trashedAt.IsZero() {
			if t, perr := time.Parse("2006-01-02", day); perr == nil {
				trashedAt = t
			} else {
				trashedAt = time.Now().UTC()
			}
		}
		entries = append(entries, TrashEntry{
			ID:           filepath.ToSlash(rel),
			OriginalPath: original,
			TrashPath:    filepath.ToSlash(path),
			TrashedAt:    trashedAt.UTC(),
			Size:         size,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("lister la corbeille : %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].TrashedAt.After(entries[j].TrashedAt) })
	return entries, nil
}

// purgeTrash supprime les entrées plus vieilles que retentionDays.
// Le calcul de l'âge utilise la date d'enregistrement (trashed_at du
// sidecar) puis, à défaut, la date du dossier jour, puis la mtime du fichier.
func purgeTrash(root string, retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	entries, err := listTrash(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		age := entry.TrashedAt
		if age.IsZero() {
			if info, err := os.Stat(entry.TrashPath); err == nil {
				age = info.ModTime().UTC()
			}
		}
		if age.Before(cutoff) {
			_ = os.Remove(entry.TrashPath)
			_ = os.Remove(entry.TrashPath + ".meta")
		}
	}
	return nil
}

// ensureDirs crée la structure minimale du coffre si elle n'existe pas.
func ensureDirs(root string) error {
	for _, dir := range []string{"notes", "assets", "templates", ".notevault"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return fmt.Errorf("créer %s : %w", dir, err)
		}
	}
	return nil
}

// copyFileAtomic copie src vers dst en passant par un fichier temporaire
// renommé à la fin. Garantit l'intégrité en cas d'interruption.
func copyFileAtomic(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(info.Mode()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// openInOS ouvre une cible (fichier ou dossier) dans le gestionnaire
// de fichiers natif. Sur Linux : xdg-open. Sur macOS : open. Sur Windows : explorer.
func openInOS(target string) error {
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("cible introuvable : %s", target)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("explorer", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Ne pas bloquer : détachement du process enfant.
	go func() { _ = cmd.Wait() }()
	return nil
}
