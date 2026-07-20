package vault

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEncryptedPayloadAuthentication(t *testing.T) {
	t.Parallel()
	key := bytes.Repeat([]byte{0x42}, 32)
	otherKey := bytes.Repeat([]byte{0x24}, 32)
	plain := []byte("---\ntitle: Secret\n---\ncontenu")

	first, err := encryptFilePayload(key, "notes/a.md", plain)
	if err != nil {
		t.Fatalf("encryptFilePayload: %v", err)
	}
	second, err := encryptFilePayload(key, "notes/a.md", plain)
	if err != nil {
		t.Fatalf("encryptFilePayload second: %v", err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("two writes used identical ciphertext")
	}
	opened, err := decryptFilePayload(key, "notes/a.md", first)
	if err != nil {
		t.Fatalf("decryptFilePayload: %v", err)
	}
	if !bytes.Equal(opened, plain) {
		t.Fatalf("round-trip = %q, want %q", opened, plain)
	}

	tests := []struct {
		name string
		key  []byte
		path string
		data func() []byte
	}{
		{name: "wrong key", key: otherKey, path: "notes/a.md", data: func() []byte { return first }},
		{name: "wrong AAD path", key: key, path: "notes/b.md", data: func() []byte { return first }},
		{name: "truncated", key: key, path: "notes/a.md", data: func() []byte { return first[:len(first)-5] }},
		{name: "tampered", key: key, path: "notes/a.md", data: func() []byte {
			value := append([]byte(nil), first...)
			value[len(value)-1] ^= 1
			return value
		}},
		{name: "unknown version", key: key, path: "notes/a.md", data: func() []byte {
			value := append([]byte(nil), first...)
			value[len(fileMagic)-1]++
			return value
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := decryptFilePayload(test.key, test.path, test.data()); err == nil {
				t.Fatal("decryptFilePayload unexpectedly succeeded")
			}
		})
	}
}

func TestServiceEncryptionLifecycle(t *testing.T) {
	root := t.TempDir()
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	note, err := svc.CreateNote("", "Confidentiel", "blank")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	note.Content = "texte secret"
	if _, err := svc.SaveNote(note); err != nil {
		t.Fatalf("SaveNote: %v", err)
	}
	if err := svc.MarkOnboardingCompleted(nil); err != nil {
		t.Fatalf("MarkOnboardingCompleted: %v", err)
	}
	if err := svc.SetDirtyBuffer(note.RelativePath, "brouillon secret", time.Time{}); err != nil {
		t.Fatalf("SetDirtyBuffer: %v", err)
	}

	const firstPassphrase = "phrase secrète initiale"
	if err := svc.EnableEncryption(firstPassphrase); err != nil {
		t.Fatalf("EnableEncryption: %v", err)
	}
	status := svc.VaultStatus()
	if status.State != VaultUnlocked || !status.EncryptionEnabled {
		t.Fatalf("status after enable = %+v", status)
	}
	notePath := filepath.Join(root, filepath.FromSlash(note.RelativePath))
	ciphertextBeforeChange, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("ReadFile encrypted note: %v", err)
	}
	if !isEncryptedPayload(ciphertextBeforeChange) || bytes.Contains(ciphertextBeforeChange, []byte("texte secret")) {
		t.Fatal("note was not encrypted on disk")
	}
	state, err := svc.LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if state.Buffer != "" || state.BufferCiphertext == "" {
		t.Fatalf("recovery buffer not migrated: %+v", state)
	}
	files, err := svc.encryptedPayloadFiles()
	if err != nil {
		t.Fatalf("encryptedPayloadFiles: %v", err)
	}
	for _, file := range files {
		stored, readErr := os.ReadFile(file.Path)
		if readErr != nil || !isEncryptedPayload(stored) {
			t.Fatalf("payload %s was not encrypted: %v", file.Path, readErr)
		}
	}
	exportPath := filepath.Join(root, "encrypted-export.zip")
	if err := svc.ExportNotes([]string{note.RelativePath}, exportPath); err != nil {
		t.Fatalf("ExportNotes encrypted: %v", err)
	}
	archive, err := zip.OpenReader(exportPath)
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	if len(archive.File) == 0 {
		t.Fatal("encrypted export is empty")
	}
	exported, err := archive.File[0].Open()
	if err != nil {
		t.Fatalf("open exported note: %v", err)
	}
	exportedMarkdown, err := io.ReadAll(exported)
	_ = exported.Close()
	_ = archive.Close()
	if err != nil || !bytes.Contains(exportedMarkdown, []byte("texte secret")) || isEncryptedPayload(exportedMarkdown) {
		t.Fatalf("export is not plaintext Markdown: %v", err)
	}
	if _, err := base64.StdEncoding.DecodeString(state.BufferCiphertext); err != nil {
		t.Fatalf("recovery ciphertext is not base64: %v", err)
	}

	const replacement = "phrase secrète remplacée"
	if err := svc.ChangePassphrase(firstPassphrase, replacement); err != nil {
		t.Fatalf("ChangePassphrase: %v", err)
	}
	ciphertextAfterChange, _ := os.ReadFile(notePath)
	if !bytes.Equal(ciphertextBeforeChange, ciphertextAfterChange) {
		t.Fatal("changing the passphrase rewrote note ciphertext")
	}

	// Simulate an interrupted activation with a controlled mixed format. The
	// next unlock must resume and encrypt the remaining plaintext file.
	plainForResume, err := decryptFilePayload(svc.vaultKey, note.RelativePath, ciphertextAfterChange)
	if err != nil {
		t.Fatalf("decrypt for resume setup: %v", err)
	}
	infoBeforeResume, _ := os.Stat(notePath)
	if err := writeAtomic(notePath, plainForResume, 0o600); err != nil {
		t.Fatalf("write mixed plaintext: %v", err)
	}
	if infoBeforeResume != nil {
		_ = os.Chtimes(notePath, infoBeforeResume.ModTime(), infoBeforeResume.ModTime())
	}
	svc.metadata.State = VaultEnabling
	if err := saveEncryptionMetadata(root, svc.metadata); err != nil {
		t.Fatalf("save interrupted metadata: %v", err)
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	locked, err := New(Options{Root: root})
	if err != nil {
		t.Fatalf("New locked: %v", err)
	}
	defer locked.Close()
	if _, err := locked.ListNotes(); !errors.Is(err, ErrVaultLocked) {
		t.Fatalf("ListNotes locked error = %v", err)
	}
	if err := locked.UnlockVault(firstPassphrase); !errors.Is(err, ErrUnlockFailed) {
		t.Fatalf("old passphrase error = %v", err)
	}
	if err := locked.UnlockVault(replacement); err != nil {
		t.Fatalf("UnlockVault: %v", err)
	}
	resumedCiphertext, _ := os.ReadFile(notePath)
	if !isEncryptedPayload(resumedCiphertext) {
		t.Fatal("interrupted activation was not resumed")
	}
	opened, err := locked.OpenNote(note.RelativePath)
	if err != nil || strings.TrimSpace(opened.Content) != "texte secret" {
		t.Fatalf("OpenNote after unlock = %+v, %v", opened, err)
	}
	history, err := locked.ListHistory(note.RelativePath)
	if err != nil || len(history) == 0 {
		t.Fatalf("ListHistory after unlock = %+v, %v", history, err)
	}
	if _, err := locked.ReadHistoryVersion(note.RelativePath, history[0].ID); err != nil {
		t.Fatalf("ReadHistoryVersion after unlock: %v", err)
	}
	snapshot, err := locked.SnapshotForStartup()
	if err != nil || snapshot.Buffer != "brouillon secret" {
		t.Fatalf("recovery after unlock = %+v, %v", snapshot, err)
	}

	movedPath := "notes/archive/confidentiel.md"
	if _, err := locked.MoveNote(note.RelativePath, movedPath); err != nil {
		t.Fatalf("MoveNote encrypted: %v", err)
	}
	movedCiphertext, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(movedPath)))
	if bytes.Equal(ciphertextAfterChange, movedCiphertext) {
		t.Fatal("encrypted rename did not re-encrypt for the new AAD path")
	}
	if _, err := decryptFilePayload(locked.vaultKey, note.RelativePath, movedCiphertext); err == nil {
		t.Fatal("moved ciphertext still authenticates at old path")
	}
	if err := locked.DeleteNote(movedPath); err != nil {
		t.Fatalf("DeleteNote encrypted: %v", err)
	}
	trash, err := locked.ListTrash()
	if err != nil || len(trash) != 1 {
		t.Fatalf("ListTrash = %+v, %v", trash, err)
	}
	trashCiphertext, _ := os.ReadFile(trash[0].TrashPath)
	if !isEncryptedPayload(trashCiphertext) {
		t.Fatal("trash did not preserve encrypted payload")
	}

	if err := locked.DisableEncryption(replacement); err != nil {
		t.Fatalf("DisableEncryption: %v", err)
	}
	trashPlaintext, _ := os.ReadFile(trash[0].TrashPath)
	if isEncryptedPayload(trashPlaintext) || !bytes.Contains(trashPlaintext, []byte("texte secret")) {
		t.Fatal("trash was not decrypted while disabling")
	}
	if _, err := locked.RestoreFromTrash(trash[0].ID); err != nil {
		t.Fatalf("RestoreFromTrash after disable: %v", err)
	}
	plainOnDisk, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(movedPath)))
	if err != nil {
		t.Fatalf("read disabled note: %v", err)
	}
	if isEncryptedPayload(plainOnDisk) || !bytes.Contains(plainOnDisk, []byte("texte secret")) {
		t.Fatal("note was not restored to plaintext")
	}
	if _, err := os.Stat(encryptionPath(root)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("encryption metadata still exists: %v", err)
	}
	state, _ = locked.LoadState()
	if state.Buffer != "brouillon secret" || state.BufferCiphertext != "" {
		t.Fatalf("recovery buffer not decrypted: %+v", state)
	}
}

func TestCorruptEncryptionMetadataStaysLocked(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := ensureDirs(root); err != nil {
		t.Fatalf("ensureDirs: %v", err)
	}
	if err := writeAtomic(encryptionPath(root), []byte(`{broken`), 0o600); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer svc.Close()
	status := svc.VaultStatus()
	if status.State != VaultLocked || !status.EncryptionEnabled {
		t.Fatalf("status = %+v", status)
	}
	if err := svc.UnlockVault("phrase secrète quelconque"); !errors.Is(err, ErrUnlockFailed) {
		t.Fatalf("UnlockVault error = %v", err)
	}
}

func TestPassphraseValidation(t *testing.T) {
	t.Parallel()
	if err := validatePassphrase("trop courte"); err == nil {
		t.Fatal("short passphrase accepted")
	}
	if err := validatePassphrase("douze caractères"); err != nil {
		t.Fatalf("valid passphrase rejected: %v", err)
	}
}
