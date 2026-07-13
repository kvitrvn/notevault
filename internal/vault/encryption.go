package vault

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

type VaultState string

const (
	VaultDisabled  VaultState = "disabled"
	VaultLocked    VaultState = "locked"
	VaultUnlocked  VaultState = "unlocked"
	VaultEnabling  VaultState = "enabling"
	VaultDisabling VaultState = "disabling"
	vaultEnabled   VaultState = "enabled"
)

const (
	encryptionVersion = 1
	minimumPassphrase = 12
	maximumPassphrase = 1024
	kdfMemoryKiB      = 64 * 1024
	kdfIterations     = 3
	kdfParallelism    = 4
)

var (
	fileMagic       = []byte{'N', 'O', 'T', 'E', 'V', 'L', 'T', 1}
	ErrVaultLocked  = errors.New("coffre verrouillé")
	ErrUnlockFailed = errors.New("impossible de déverrouiller le coffre")
)

type KDFParameters struct {
	Algorithm   string `json:"algorithm"`
	MemoryKiB   uint32 `json:"memoryKiB"`
	Iterations  uint32 `json:"iterations"`
	Parallelism uint8  `json:"parallelism"`
	Salt        string `json:"salt"`
}

type encryptionMetadata struct {
	Version    int           `json:"version"`
	State      VaultState    `json:"state"`
	KDF        KDFParameters `json:"kdf"`
	WrappedKey string        `json:"wrappedKey"`
	Warnings   []string      `json:"warnings,omitempty"`
	UpdatedAt  time.Time     `json:"updatedAt"`
}

type VaultStatusInfo struct {
	State             VaultState `json:"state"`
	EncryptionEnabled bool       `json:"encryptionEnabled"`
	Warnings          []string   `json:"warnings"`
	MigrationCurrent  int        `json:"migrationCurrent"`
	MigrationTotal    int        `json:"migrationTotal"`
}

type payloadMigrationFile struct {
	Path    string
	AADPath string
}

func validatePassphrase(passphrase string) error {
	length := len([]rune(passphrase))
	if length < minimumPassphrase {
		return fmt.Errorf("la phrase secrète doit contenir au moins %d caractères", minimumPassphrase)
	}
	if len(passphrase) > maximumPassphrase {
		return errors.New("phrase secrète trop longue")
	}
	return nil
}

func defaultKDF(salt []byte) KDFParameters {
	return KDFParameters{
		Algorithm:   "argon2id",
		MemoryKiB:   kdfMemoryKiB,
		Iterations:  kdfIterations,
		Parallelism: kdfParallelism,
		Salt:        base64.StdEncoding.EncodeToString(salt),
	}
}

func deriveEnvelopeKey(passphrase string, params KDFParameters) ([]byte, error) {
	if params.Algorithm != "argon2id" || params.MemoryKiB < 8*1024 || params.MemoryKiB > 1024*1024 ||
		params.Iterations == 0 || params.Iterations > 20 || params.Parallelism == 0 || params.Parallelism > 32 {
		return nil, ErrUnlockFailed
	}
	salt, err := base64.StdEncoding.DecodeString(params.Salt)
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return nil, ErrUnlockFailed
	}
	return argon2.IDKey([]byte(passphrase), salt, params.Iterations, params.MemoryKiB, params.Parallelism, 32), nil
}

func sealRandomNonce(key, plaintext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("initialiser AES : %w", err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, fmt.Errorf("initialiser AES-GCM : %w", err)
	}
	return aead.Seal(nil, nil, plaintext, aad), nil
}

func openRandomNonce(key, ciphertext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrUnlockFailed
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil || len(ciphertext) < aead.Overhead() {
		return nil, ErrUnlockFailed
	}
	plaintext, err := aead.Open(nil, nil, ciphertext, aad)
	if err != nil {
		return nil, ErrUnlockFailed
	}
	return plaintext, nil
}

func payloadAAD(relativePath string) []byte {
	return []byte(fmt.Sprintf("notevault:%d:%s", encryptionVersion, filepath.ToSlash(relativePath)))
}

func encryptFilePayload(key []byte, relativePath string, plaintext []byte) ([]byte, error) {
	ciphertext, err := sealRandomNonce(key, plaintext, payloadAAD(relativePath))
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(fileMagic)+len(ciphertext))
	out = append(out, fileMagic...)
	out = append(out, ciphertext...)
	return out, nil
}

func decryptFilePayload(key []byte, relativePath string, stored []byte) ([]byte, error) {
	if !isEncryptedPayload(stored) {
		return nil, errors.New("payload non chiffré")
	}
	return openRandomNonce(key, stored[len(fileMagic):], payloadAAD(relativePath))
}

func isEncryptedPayload(stored []byte) bool {
	return len(stored) >= len(fileMagic) && string(stored[:len(fileMagic)]) == string(fileMagic)
}

func encryptionPath(root string) string {
	return filepath.Join(root, ".notevault", "encryption.json")
}

func loadEncryptionMetadata(root string) (*encryptionMetadata, error) {
	raw, err := os.ReadFile(encryptionPath(root))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lire encryption.json : %w", err)
	}
	var metadata encryptionMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil || metadata.Version != encryptionVersion || metadata.WrappedKey == "" ||
		(metadata.State != vaultEnabled && metadata.State != VaultEnabling && metadata.State != VaultDisabling) {
		return nil, ErrUnlockFailed
	}
	return &metadata, nil
}

func saveEncryptionMetadata(root string, metadata *encryptionMetadata) error {
	metadata.UpdatedAt = nowUTC()
	raw, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encoder encryption.json : %w", err)
	}
	if err := writeAtomic(encryptionPath(root), raw, 0o600); err != nil {
		return fmt.Errorf("écrire encryption.json : %w", err)
	}
	return nil
}

func wrapVaultKey(envelopeKey, vaultKey []byte) (string, error) {
	wrapped, err := sealRandomNonce(envelopeKey, vaultKey, []byte("notevault:1:vault-key"))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(wrapped), nil
}

func unwrapVaultKey(envelopeKey []byte, encoded string) ([]byte, error) {
	wrapped, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, ErrUnlockFailed
	}
	key, err := openRandomNonce(envelopeKey, wrapped, []byte("notevault:1:vault-key"))
	if err != nil || len(key) != 32 {
		return nil, ErrUnlockFailed
	}
	return key, nil
}

func randomBytes(size int) ([]byte, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return nil, fmt.Errorf("générer des octets aléatoires : %w", err)
	}
	return value, nil
}

func zeroBytes(value []byte) {
	for i := range value {
		value[i] = 0
	}
}

func (s *Service) VaultStatus() VaultStatusInfo {
	s.securityMu.RLock()
	defer s.securityMu.RUnlock()
	status := VaultStatusInfo{
		State:            s.vaultState,
		Warnings:         append([]string(nil), s.encryptionWarnings...),
		MigrationCurrent: s.migrationCurrent,
		MigrationTotal:   s.migrationTotal,
	}
	status.EncryptionEnabled = s.metadata != nil || s.metadataInvalid
	return status
}

func (s *Service) requireUnlocked() error {
	s.securityMu.RLock()
	defer s.securityMu.RUnlock()
	if s.vaultState == VaultLocked || s.vaultState == VaultEnabling || s.vaultState == VaultDisabling {
		return ErrVaultLocked
	}
	return nil
}

func (s *Service) EnableEncryption(passphrase string) error {
	if err := validatePassphrase(passphrase); err != nil {
		return err
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.securityMu.Lock()
	if s.vaultState != VaultDisabled || s.metadata != nil {
		s.securityMu.Unlock()
		return errors.New("le chiffrement est déjà activé")
	}
	salt, err := randomBytes(16)
	if err != nil {
		s.securityMu.Unlock()
		return err
	}
	vaultKey, err := randomBytes(32)
	if err != nil {
		s.securityMu.Unlock()
		return err
	}
	params := defaultKDF(salt)
	envelopeKey, err := deriveEnvelopeKey(passphrase, params)
	if err != nil {
		zeroBytes(vaultKey)
		s.securityMu.Unlock()
		return err
	}
	wrapped, err := wrapVaultKey(envelopeKey, vaultKey)
	zeroBytes(envelopeKey)
	if err != nil {
		zeroBytes(vaultKey)
		s.securityMu.Unlock()
		return err
	}
	metadata := &encryptionMetadata{Version: encryptionVersion, State: VaultEnabling, KDF: params, WrappedKey: wrapped}
	s.metadata = metadata
	s.vaultKey = vaultKey
	s.vaultState = VaultEnabling
	if err := saveEncryptionMetadata(s.root, metadata); err != nil {
		zeroBytes(s.vaultKey)
		s.vaultKey = nil
		s.vaultState = VaultLocked
		s.securityMu.Unlock()
		if index, ok := s.index.(*memoryIndex); ok {
			index.reset()
		}
		return err
	}
	s.securityMu.Unlock()
	s.stopWatcher()
	if err := s.resumeMigration(vaultKey, VaultEnabling); err != nil {
		s.failMigration()
		return err
	}
	return nil
}

func (s *Service) UnlockVault(passphrase string) error {
	if len(passphrase) > maximumPassphrase {
		return ErrUnlockFailed
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.securityMu.RLock()
	metadata := s.metadata
	state := s.vaultState
	invalid := s.metadataInvalid
	s.securityMu.RUnlock()
	if invalid {
		return ErrUnlockFailed
	}
	if metadata == nil || state != VaultLocked {
		return errors.New("le coffre n’est pas verrouillé")
	}
	envelopeKey, err := deriveEnvelopeKey(passphrase, metadata.KDF)
	if err != nil {
		return ErrUnlockFailed
	}
	vaultKey, err := unwrapVaultKey(envelopeKey, metadata.WrappedKey)
	zeroBytes(envelopeKey)
	if err != nil {
		return ErrUnlockFailed
	}
	if metadata.State == VaultEnabling || metadata.State == VaultDisabling {
		s.securityMu.Lock()
		s.vaultState = metadata.State
		s.vaultKey = vaultKey
		s.securityMu.Unlock()
		if err := s.resumeMigration(vaultKey, metadata.State); err != nil {
			s.failMigration()
			return ErrUnlockFailed
		}
		return nil
	}
	s.securityMu.Lock()
	s.vaultKey = vaultKey
	s.vaultState = VaultUnlocked
	s.securityMu.Unlock()
	if err := s.IndexNow(s.BootstrapContext(), nil); err != nil {
		s.addEncryptionWarning(err.Error())
	}
	if err := s.startWatcher(); err != nil {
		s.addEncryptionWarning(err.Error())
	}
	return nil
}

func (s *Service) ChangePassphrase(current, replacement string) error {
	if err := validatePassphrase(replacement); err != nil {
		return err
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	s.securityMu.RLock()
	var metadata encryptionMetadata
	if s.metadata != nil {
		metadata = *s.metadata
		metadata.Warnings = append([]string(nil), s.metadata.Warnings...)
	}
	s.securityMu.RUnlock()
	if metadata.WrappedKey == "" {
		return errors.New("le chiffrement n’est pas activé")
	}
	currentKey, err := deriveEnvelopeKey(current, metadata.KDF)
	if err != nil {
		return ErrUnlockFailed
	}
	vaultKey, err := unwrapVaultKey(currentKey, metadata.WrappedKey)
	zeroBytes(currentKey)
	if err != nil {
		return ErrUnlockFailed
	}
	salt, err := randomBytes(16)
	if err != nil {
		zeroBytes(vaultKey)
		return err
	}
	params := defaultKDF(salt)
	replacementKey, err := deriveEnvelopeKey(replacement, params)
	if err != nil {
		zeroBytes(vaultKey)
		return err
	}
	wrapped, err := wrapVaultKey(replacementKey, vaultKey)
	zeroBytes(replacementKey)
	zeroBytes(vaultKey)
	if err != nil {
		return err
	}
	metadata.KDF = params
	metadata.WrappedKey = wrapped
	s.securityMu.Lock()
	metadata.Warnings = append([]string(nil), s.encryptionWarnings...)
	if err := saveEncryptionMetadata(s.root, &metadata); err != nil {
		s.securityMu.Unlock()
		return err
	}
	s.metadata = &metadata
	s.securityMu.Unlock()
	return nil
}

func (s *Service) DisableEncryption(passphrase string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.securityMu.RLock()
	metadata := s.metadata
	s.securityMu.RUnlock()
	if metadata == nil {
		return errors.New("le chiffrement n’est pas activé")
	}
	envelopeKey, err := deriveEnvelopeKey(passphrase, metadata.KDF)
	if err != nil {
		return ErrUnlockFailed
	}
	vaultKey, err := unwrapVaultKey(envelopeKey, metadata.WrappedKey)
	zeroBytes(envelopeKey)
	if err != nil {
		return ErrUnlockFailed
	}
	s.securityMu.Lock()
	metadata.State = VaultDisabling
	if err := saveEncryptionMetadata(s.root, metadata); err != nil {
		s.securityMu.Unlock()
		zeroBytes(vaultKey)
		return err
	}
	s.vaultState = VaultDisabling
	s.vaultKey = vaultKey
	s.securityMu.Unlock()
	s.stopWatcher()
	if err := s.resumeMigration(vaultKey, VaultDisabling); err != nil {
		s.failMigration()
		return err
	}
	return nil
}

func (s *Service) failMigration() {
	s.securityMu.Lock()
	zeroBytes(s.vaultKey)
	s.vaultKey = nil
	s.vaultState = VaultLocked
	s.securityMu.Unlock()
	if index, ok := s.index.(*memoryIndex); ok {
		index.reset()
	}
}

func (s *Service) resumeMigration(key []byte, direction VaultState) error {
	files, err := s.encryptedPayloadFiles()
	if err != nil {
		return err
	}
	s.securityMu.Lock()
	s.migrationCurrent = 0
	s.migrationTotal = len(files) + 1
	s.securityMu.Unlock()
	for n, file := range files {
		rel := file.AADPath
		raw, err := os.ReadFile(file.Path)
		if err != nil {
			return fmt.Errorf("lire %s pendant la migration : %w", rel, err)
		}
		info, statErr := os.Stat(file.Path)
		var converted []byte
		switch direction {
		case VaultEnabling:
			if isEncryptedPayload(raw) {
				if _, err := decryptFilePayload(key, rel, raw); err != nil {
					return fmt.Errorf("valider %s : %w", rel, err)
				}
				converted = raw
			} else {
				converted, err = encryptFilePayload(key, rel, raw)
			}
		case VaultDisabling:
			if isEncryptedPayload(raw) {
				converted, err = decryptFilePayload(key, rel, raw)
			} else {
				converted = raw
			}
		}
		if err != nil {
			return fmt.Errorf("convertir %s : %w", rel, err)
		}
		if !bytes.Equal(converted, raw) {
			if err := writeAtomic(file.Path, converted, 0o600); err != nil {
				return err
			}
			if statErr == nil {
				_ = os.Chtimes(file.Path, info.ModTime(), info.ModTime())
			}
		}
		s.securityMu.Lock()
		s.migrationCurrent = n + 1
		s.securityMu.Unlock()
	}
	if err := s.migrateRecovery(key, direction); err != nil {
		return err
	}
	if direction == VaultEnabling {
		s.securityMu.Lock()
		s.metadata.State = vaultEnabled
		if err := saveEncryptionMetadata(s.root, s.metadata); err != nil {
			s.securityMu.Unlock()
			return err
		}
		s.vaultState = VaultUnlocked
		s.vaultKey = key
		s.migrationCurrent = s.migrationTotal
		s.securityMu.Unlock()
	} else {
		if err := os.Remove(encryptionPath(s.root)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		s.securityMu.Lock()
		zeroBytes(s.vaultKey)
		s.vaultKey = nil
		s.metadata = nil
		s.vaultState = VaultDisabled
		s.migrationCurrent = s.migrationTotal
		s.securityMu.Unlock()
	}
	if err := s.IndexNow(s.BootstrapContext(), nil); err != nil {
		s.addEncryptionWarning(err.Error())
	}
	if err := s.startWatcher(); err != nil {
		s.addEncryptionWarning(err.Error())
	}
	return nil
}

func (s *Service) encryptedPayloadFiles() ([]payloadMigrationFile, error) {
	roots := []string{filepath.Join(s.root, "notes"), filepath.Join(s.root, ".notevault", "history")}
	files := make([]payloadMigrationFile, 0)
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if !entry.IsDir() && strings.EqualFold(filepath.Ext(path), ".md") {
				rel, err := filepath.Rel(s.root, path)
				if err != nil {
					return err
				}
				files = append(files, payloadMigrationFile{Path: path, AADPath: filepath.ToSlash(rel)})
			}
			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	trash, err := listTrash(s.root)
	if err != nil {
		return nil, err
	}
	for _, entry := range trash {
		if err := s.validateNoteRelPath(entry.OriginalPath); err != nil {
			return nil, fmt.Errorf("chemin d’origine de corbeille invalide pour %s", entry.ID)
		}
		files = append(files, payloadMigrationFile{Path: entry.TrashPath, AADPath: entry.OriginalPath})
	}
	return files, nil
}

func (s *Service) migrateRecovery(key []byte, direction VaultState) error {
	state, err := s.LoadState()
	if err != nil {
		return err
	}
	if direction == VaultEnabling && state.Buffer != "" {
		ciphertext, err := encryptFilePayload(key, ".notevault/state.json#buffer", []byte(state.Buffer))
		if err != nil {
			return err
		}
		state.BufferCiphertext = base64.StdEncoding.EncodeToString(ciphertext)
		state.Buffer = ""
	}
	if direction == VaultDisabling && state.BufferCiphertext != "" {
		ciphertext, err := base64.StdEncoding.DecodeString(state.BufferCiphertext)
		if err != nil {
			return ErrUnlockFailed
		}
		plaintext, err := decryptFilePayload(key, ".notevault/state.json#buffer", ciphertext)
		if err != nil {
			return ErrUnlockFailed
		}
		state.Buffer = string(plaintext)
		state.BufferCiphertext = ""
	}
	return s.SaveState(state)
}

func (s *Service) addEncryptionWarning(message string) {
	s.securityMu.Lock()
	defer s.securityMu.Unlock()
	for _, warning := range s.encryptionWarnings {
		if warning == message {
			return
		}
	}
	s.encryptionWarnings = append(s.encryptionWarnings, message)
	if s.metadata != nil {
		s.metadata.Warnings = append([]string(nil), s.encryptionWarnings...)
		_ = saveEncryptionMetadata(s.root, s.metadata)
	}
}

func (s *Service) readPayload(relativePath string) ([]byte, error) {
	var err error
	relativePath, err = normalizeVaultRelative(relativePath)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(s.root, filepath.FromSlash(relativePath))
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s.securityMu.RLock()
	state := s.vaultState
	key := append([]byte(nil), s.vaultKey...)
	s.securityMu.RUnlock()
	defer zeroBytes(key)
	if state == VaultLocked || state == VaultEnabling || state == VaultDisabling {
		return nil, ErrVaultLocked
	}
	if state == VaultDisabled {
		if isEncryptedPayload(raw) {
			return nil, errors.New("fichier chiffré inattendu dans un coffre non chiffré")
		}
		return raw, nil
	}
	if !isEncryptedPayload(raw) {
		return nil, errors.New("fichier clair refusé dans un coffre chiffré")
	}
	return decryptFilePayload(key, relativePath, raw)
}

func (s *Service) writePayload(relativePath string, plaintext []byte, perm os.FileMode) error {
	var err error
	relativePath, err = normalizeVaultRelative(relativePath)
	if err != nil {
		return err
	}
	s.securityMu.RLock()
	state := s.vaultState
	key := append([]byte(nil), s.vaultKey...)
	s.securityMu.RUnlock()
	defer zeroBytes(key)
	if state == VaultLocked || state == VaultEnabling || state == VaultDisabling {
		return ErrVaultLocked
	}
	stored := plaintext
	if state == VaultUnlocked {
		var err error
		stored, err = encryptFilePayload(key, relativePath, plaintext)
		if err != nil {
			return err
		}
		perm = 0o600
	}
	return writeAtomic(filepath.Join(s.root, filepath.FromSlash(relativePath)), stored, perm)
}

func normalizeVaultRelative(relativePath string) (string, error) {
	local := filepath.FromSlash(relativePath)
	if relativePath == "" || filepath.IsAbs(local) || !filepath.IsLocal(local) {
		return "", errors.New("chemin relatif au coffre invalide")
	}
	clean := filepath.Clean(local)
	if clean == "." {
		return "", errors.New("chemin relatif au coffre invalide")
	}
	return filepath.ToSlash(clean), nil
}
