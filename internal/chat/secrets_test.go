package chat

import (
	"errors"
	"strings"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

type fakeKeyringBackend struct {
	values    map[string]string
	getErr    error
	setErr    error
	deleteErr error
}

func (f *fakeKeyringBackend) Get(service, user string) (string, error) {
	if service != keyringService {
		return "", errors.New("unexpected service")
	}
	if f.getErr != nil {
		return "", f.getErr
	}
	value, ok := f.values[user]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

func (f *fakeKeyringBackend) Set(service, user, password string) error {
	if service != keyringService {
		return errors.New("unexpected service")
	}
	if f.setErr != nil {
		return f.setErr
	}
	if f.values == nil {
		f.values = make(map[string]string)
	}
	f.values[user] = password
	return nil
}

func (f *fakeKeyringBackend) Delete(service, user string) error {
	if service != keyringService {
		return errors.New("unexpected service")
	}
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if _, ok := f.values[user]; !ok {
		return keyring.ErrNotFound
	}
	delete(f.values, user)
	return nil
}

func TestKeyringStoreLifecycle(t *testing.T) {
	t.Parallel()
	backend := &fakeKeyringBackend{values: make(map[string]string)}
	store := newKeyringStore(backend)

	if err := store.Available(); err != nil {
		t.Fatalf("Available: %v", err)
	}
	if _, err := store.Get(ProviderOpenAI); !errors.Is(err, ErrAPIKeyNotFound) {
		t.Fatalf("Get missing = %v", err)
	}
	if err := store.Set(ProviderOpenAI, "first-secret"); err != nil {
		t.Fatalf("Set first: %v", err)
	}
	if err := store.Set(ProviderOpenAI, "replacement-secret"); err != nil {
		t.Fatalf("Set replacement: %v", err)
	}
	got, err := store.Get(ProviderOpenAI)
	if err != nil || got != "replacement-secret" {
		t.Fatalf("Get replacement = %q, %v", got, err)
	}
	if err := store.Delete(ProviderOpenAI); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete(ProviderOpenAI); err != nil {
		t.Fatalf("Delete idempotent: %v", err)
	}
}

func TestKeyringStoreMapsUnavailableWithoutLeakingSecret(t *testing.T) {
	t.Parallel()
	const secret = "never-print-this-key"
	tests := []struct {
		name    string
		backend *fakeKeyringBackend
		call    func(*KeyringStore) error
	}{
		{name: "locked read", backend: &fakeKeyringBackend{getErr: errors.New("collection locked")}, call: func(store *KeyringStore) error { _, err := store.Get(ProviderMistral); return err }},
		{name: "service failure write", backend: &fakeKeyringBackend{setErr: errors.New("dbus unavailable")}, call: func(store *KeyringStore) error { return store.Set(ProviderMistral, secret) }},
		{name: "service failure delete", backend: &fakeKeyringBackend{deleteErr: errors.New("dbus unavailable")}, call: func(store *KeyringStore) error { return store.Delete(ProviderMistral) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.call(newKeyringStore(test.backend))
			if !errors.Is(err, ErrKeyringUnavailable) {
				t.Fatalf("error = %v, want ErrKeyringUnavailable", err)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatal("l’erreur contient la clé")
			}
		})
	}
}

func TestKeyringStoreAvailabilityMapsLockedService(t *testing.T) {
	t.Parallel()
	store := newKeyringStore(&fakeKeyringBackend{getErr: errors.New("collection locked")})
	if err := store.Available(); !errors.Is(err, ErrKeyringUnavailable) {
		t.Fatalf("Available error = %v, want ErrKeyringUnavailable", err)
	}
}

func TestKeyringStoreRejectsInvalidProviderAndKey(t *testing.T) {
	t.Parallel()
	store := newKeyringStore(&fakeKeyringBackend{values: make(map[string]string)})
	for _, test := range []struct {
		name     string
		provider Provider
		key      string
	}{
		{name: "ollama", provider: ProviderOllama, key: "secret"},
		{name: "unknown", provider: "custom", key: "secret"},
		{name: "empty", provider: ProviderOpenRouter},
		{name: "oversized", provider: ProviderOpenRouter, key: strings.Repeat("k", maxAPIKeyBytes+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := store.Set(test.provider, test.key); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("Set error = %v", err)
			}
		})
	}
}
