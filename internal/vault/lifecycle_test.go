package vault

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateExistingVaultDoesNotMutateInvalidDirectory(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "document.txt")
	if err := os.WriteFile(marker, []byte("inchangé"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateExistingVault(root); !errors.Is(err, ErrInvalidVault) {
		t.Fatalf("error = %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "document.txt" {
		t.Fatalf("dossier modifié: %+v", entries)
	}
}

func TestValidateExistingVaultRejectsSymlinkedNotesDirectory(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".notevault"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "notes")); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateExistingVault(root); !errors.Is(err, ErrInvalidVault) {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateVaultName(t *testing.T) {
	for _, tc := range []struct {
		name  string
		valid bool
	}{
		{name: "Mes notes", valid: true},
		{name: ""},
		{name: "."},
		{name: ".."},
		{name: "a/b"},
		{name: "a\\b"},
		{name: " espace"},
		{name: "ligne\nnouvelle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateVaultName(tc.name)
			if (err == nil) != tc.valid {
				t.Fatalf("ValidateVaultName(%q) = %v", tc.name, err)
			}
		})
	}
}
