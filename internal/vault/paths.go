package vault

import (
	"fmt"
	"os"
	"path/filepath"
)

func defaultVaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("déterminer le dossier utilisateur : %w", err)
	}
	// Le coffre est un dossier personnel, lisible sans l'application.
	return filepath.Join(home, "NoteVault"), nil
}
