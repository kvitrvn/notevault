package chat

import (
	"errors"
	"strings"

	keyring "github.com/zalando/go-keyring"
)

const keyringService = "io.github.kvitrvn.NoteVault.chat"

var (
	ErrAPIKeyNotFound     = errors.New("aucune clé API enregistrée pour ce fournisseur")
	ErrKeyringUnavailable = errors.New("le trousseau système est verrouillé ou indisponible")
)

// SecretStore keeps remote provider credentials outside the vault and global
// configuration. Implementations must never persist secrets in ordinary files.
type SecretStore interface {
	Available() error
	Get(provider Provider) (string, error)
	Set(provider Provider, apiKey string) error
	Delete(provider Provider) error
}

type keyringBackend interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

type systemKeyringBackend struct{}

func (systemKeyringBackend) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

func (systemKeyringBackend) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

func (systemKeyringBackend) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

type KeyringStore struct {
	backend keyringBackend
}

func NewKeyringStore() *KeyringStore {
	return &KeyringStore{backend: systemKeyringBackend{}}
}

func newKeyringStore(backend keyringBackend) *KeyringStore {
	return &KeyringStore{backend: backend}
}

// Available probes only the fixed NoteVault entries. A missing entry proves
// that the system service answered and is therefore considered available.
func (s *KeyringStore) Available() error {
	for _, provider := range remoteProviders() {
		_, err := s.backend.Get(keyringService, string(provider))
		if err == nil || errors.Is(err, keyring.ErrNotFound) {
			return nil
		}
	}
	return ErrKeyringUnavailable
}

func (s *KeyringStore) Get(provider Provider) (string, error) {
	if !provider.IsRemote() {
		return "", ErrInvalidRequest
	}
	apiKey, err := s.backend.Get(keyringService, string(provider))
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrAPIKeyNotFound
	}
	if err != nil {
		return "", ErrKeyringUnavailable
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", ErrAPIKeyNotFound
	}
	return apiKey, nil
}

func (s *KeyringStore) Set(provider Provider, apiKey string) error {
	apiKey = strings.TrimSpace(apiKey)
	if err := ValidateAPIKey(provider, apiKey); err != nil {
		return err
	}
	if err := s.backend.Set(keyringService, string(provider), apiKey); err != nil {
		return ErrKeyringUnavailable
	}
	return nil
}

func (s *KeyringStore) Delete(provider Provider) error {
	if !provider.IsRemote() {
		return ErrInvalidRequest
	}
	if err := s.backend.Delete(keyringService, string(provider)); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return ErrKeyringUnavailable
	}
	return nil
}

func remoteProviders() []Provider {
	return []Provider{ProviderOpenAI, ProviderMistral, ProviderOpenRouter}
}
