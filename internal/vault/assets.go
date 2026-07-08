package vault

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SaveAsset stocke des données binaires (image, pièce jointe) dans
// <root>/assets/YYYY/MM/<uuid>.<ext>. Retourne le chemin relatif
// utilisable dans le contenu Markdown (ex: "assets/2026/07/abc.png").
func (s *Service) SaveAsset(data []byte, filename string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("aucune donnée à enregistrer")
	}
	ext := sanitizeExt(filename)
	if ext == "" {
		return "", fmt.Errorf("extension de fichier manquante ou non supportée")
	}
	now := nowUTC()
	year := now.Format("2006")
	month := now.Format("01")
	dir := filepath.Join(s.root, "assets", year, month)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("préparer le dossier assets : %w", err)
	}
	uuid, err := randomID(16)
	if err != nil {
		return "", err
	}
	relPath := filepath.ToSlash(filepath.Join("assets", year, month, uuid+ext))
	dest := filepath.Join(s.root, relPath)
	if err := writeAtomic(dest, data, 0o644); err != nil {
		return "", err
	}
	return relPath, nil
}

// ImportAssetFromFilePath copie un fichier existant sur le disque dans le
// coffre. Utilisé par le frontend pour les drops de fichiers depuis un
// explorateur : sur WebKit Linux, le navigateur expose le fichier comme
// une URL `file://` dans le dataTransfer (pas comme un File direct), et
// le fetch JS de `file://` est bloqué par CORS. On lit donc le fichier
// côté Go (qui n'a pas cette restriction) et on le stocke comme un asset.
//
// L'extension est validée contre la whitelist. Le fichier source doit
// être lisible (pas de restriction de chemin : on importe depuis
// n'importe où sur le système, comme le ferait un explorateur).
func (s *Service) ImportAssetFromFilePath(absolutePath string) (string, error) {
	clean := filepath.Clean(absolutePath)
	info, err := os.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("accéder au fichier source : %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("le chemin pointe sur un dossier")
	}
	if info.Size() > 50*1024*1024 {
		return "", fmt.Errorf("fichier trop volumineux (>50 MB)")
	}
	ext := sanitizeExt(clean)
	if ext == "" {
		return "", fmt.Errorf("extension non supportée")
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		return "", fmt.Errorf("lire le fichier : %w", err)
	}
	return s.SaveAsset(data, filepath.Base(clean))
}

// sanitizeExt extrait l'extension d'un nom de fichier et la contraint
// à une liste blanche raisonnable (sécurité : évite qu'un utilisateur
// envoie un .exe par accident).
func sanitizeExt(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	ext = strings.TrimPrefix(ext, ".")
	switch ext {
	case "png", "jpg", "jpeg", "gif", "webp", "svg", "pdf", "mp3", "mp4",
		"webm", "ogg", "wav", "txt", "md":
		return "." + ext
	}
	return ""
}

func randomID(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// nowMonthDirectory retourne le dossier "YYYY/MM" pour la date donnée.
// Helper exporté pour les tests.
func nowMonthDirectory(t time.Time) string {
	return t.Format("2006") + "/" + t.Format("01")
}
