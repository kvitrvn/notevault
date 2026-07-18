package chat

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakePrivacySession struct {
	mapping map[string]string
}

type fakePrivacy struct{}

func (fakePrivacy) NewSession() privacySession {
	return &fakePrivacySession{mapping: make(map[string]string)}
}

func (fakePrivacy) Anonymize(_ context.Context, raw privacySession, text string) (privacyResult, error) {
	session := raw.(*fakePrivacySession)
	entities := []DetectedEntity{}
	for original, placeholder := range map[string]string{"Alice Dupont": "[PERSON_1]", "Paris": "[LOCATION_1]"} {
		if strings.Contains(text, original) {
			text = strings.ReplaceAll(text, original, placeholder)
			session.mapping[placeholder] = original
			entities = append(entities, DetectedEntity{Type: "PER", Placeholder: placeholder, Confidence: 1})
		}
	}
	return privacyResult{Text: text, Entities: entities}, nil
}

func (fakePrivacy) Deanonymize(raw privacySession, text string) string {
	for placeholder, original := range raw.(*fakePrivacySession).mapping {
		text = strings.ReplaceAll(text, placeholder, original)
	}
	return text
}

type fakeCompleter struct {
	answer   string
	config   completionConfig
	messages []safeMessage
}

type fakeSecretStore struct {
	keys           map[Provider]string
	availableErr   error
	availableCalls int
	getCalls       int
	setCalls       int
	deleteCalls    int
}

func (f *fakeSecretStore) Available() error {
	f.availableCalls++
	return f.availableErr
}

func (f *fakeSecretStore) Get(provider Provider) (string, error) {
	f.getCalls++
	key := f.keys[provider]
	if key == "" {
		return "", ErrAPIKeyNotFound
	}
	return key, nil
}

func (f *fakeSecretStore) Set(provider Provider, apiKey string) error {
	f.setCalls++
	if f.keys == nil {
		f.keys = make(map[Provider]string)
	}
	f.keys[provider] = apiKey
	return nil
}

func (f *fakeSecretStore) Delete(provider Provider) error {
	f.deleteCalls++
	delete(f.keys, provider)
	return nil
}

type blockingCompleter struct {
	started chan struct{}
}

func (b *blockingCompleter) Complete(ctx context.Context, _ completionConfig, _ []safeMessage) (string, error) {
	close(b.started)
	<-ctx.Done()
	return "", ctx.Err()
}

func (f *fakeCompleter) Complete(_ context.Context, config completionConfig, messages []safeMessage) (string, error) {
	f.config = config
	f.messages = append([]safeMessage(nil), messages...)
	return f.answer, nil
}

func TestServicePreviewThenSendKeepsCleartextLocal(t *testing.T) {
	t.Parallel()
	completion := &fakeCompleter{answer: "[PERSON_1] travaille à [LOCATION_1] [SOURCE_1]."}
	secrets := &fakeSecretStore{keys: map[Provider]string{ProviderOpenAI: "stored-secret"}}
	service := newService(fakePrivacy{}, completion, secrets)
	notes := []Note{{
		Path:    "notes/Alice Dupont.md",
		Title:   "Dossier Alice Dupont",
		Content: "# Profil\n\nAlice Dupont travaille à Paris sur le projet Atlas.",
	}}
	request := PrepareRequest{
		NotePaths: []string{notes[0].Path},
		Question:  "Où travaille Alice Dupont ?",
		Provider:  ProviderOpenAI,
		Model:     "test-model",
	}

	preview, err := service.Prepare(context.Background(), request, notes)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if strings.Contains(preview.OutboundText, "Alice Dupont") || strings.Contains(preview.OutboundText, notes[0].Path) {
		t.Fatalf("l'aperçu sortant contient du texte local en clair : %q", preview.OutboundText)
	}
	if !strings.Contains(preview.OutboundText, "[PERSON_1]") || !strings.Contains(preview.OutboundText, "[SOURCE_1]") {
		t.Fatalf("aperçu anonymisé incomplet : %q", preview.OutboundText)
	}

	response, err := service.Send(context.Background(), SendRequest{PreviewID: preview.ID, APIKey: "secret-in-memory"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if response.Answer != "Alice Dupont travaille à Paris [SOURCE_1]." {
		t.Fatalf("Answer = %q", response.Answer)
	}
	if completion.config.APIKey != "secret-in-memory" {
		t.Fatalf("clé non transmise au client de test")
	}
	if secrets.getCalls != 0 {
		t.Fatalf("clé enregistrée consultée malgré la clé ponctuelle: %d", secrets.getCalls)
	}
	for _, message := range completion.messages {
		if strings.Contains(message.Content, "Alice Dupont") || strings.Contains(message.Content, notes[0].Path) {
			t.Fatalf("message fournisseur en clair : %q", message.Content)
		}
	}
	if len(response.Citations) == 0 || response.Citations[0].Path != notes[0].Path {
		t.Fatalf("citations locales = %+v", response.Citations)
	}
}

func TestServiceRejectsExpiredPreview(t *testing.T) {
	t.Parallel()
	service := newService(fakePrivacy{}, &fakeCompleter{answer: "ok"}, &fakeSecretStore{})
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	notes := []Note{{Path: "notes/a.md", Title: "A", Content: "# A\n\nContenu utile."}}
	preview, err := service.Prepare(context.Background(), PrepareRequest{
		NotePaths: []string{"notes/a.md"}, Question: "Contenu ?", Provider: ProviderOllama, Model: "local",
	}, notes)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	now = now.Add(previewLifetime + time.Second)
	if _, err := service.Send(context.Background(), SendRequest{PreviewID: preview.ID}); err != ErrPreviewExpired {
		t.Fatalf("Send error = %v, want ErrPreviewExpired", err)
	}
}

func TestServiceCloseCancelsPendingCompletion(t *testing.T) {
	t.Parallel()
	completion := &blockingCompleter{started: make(chan struct{})}
	service := newService(fakePrivacy{}, completion, &fakeSecretStore{})
	preview, err := service.Prepare(context.Background(), PrepareRequest{
		NotePaths: []string{"notes/a.md"}, Question: "Contenu ?", Provider: ProviderOllama, Model: "local",
	}, []Note{{Path: "notes/a.md", Title: "A", Content: "# A\n\nContenu utile."}})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, sendErr := service.Send(context.Background(), SendRequest{PreviewID: preview.ID})
		done <- sendErr
	}()
	<-completion.started
	service.Close()

	select {
	case sendErr := <-done:
		if sendErr == nil {
			t.Fatal("Send a réussi après la fermeture du service")
		}
	case <-time.After(time.Second):
		t.Fatal("la fermeture n’a pas annulé l’appel au modèle")
	}
}

func TestValidatePrepareRequestLimitsSelection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		request PrepareRequest
		notes   []Note
	}{
		{name: "missing notes", request: PrepareRequest{Question: "q", Provider: ProviderOllama, Model: "m"}},
		{name: "missing model", request: PrepareRequest{Question: "q", Provider: ProviderOllama, NotePaths: []string{"a"}}, notes: []Note{{Path: "a", Content: "x"}}},
		{name: "model too long", request: PrepareRequest{Question: "q", Provider: ProviderOllama, Model: strings.Repeat("m", maxModelRunes+1), NotePaths: []string{"a"}}, notes: []Note{{Path: "a", Content: "x"}}},
		{name: "duplicate note", request: PrepareRequest{Question: "q", Provider: ProviderOllama, Model: "m", NotePaths: []string{"a", "a"}}, notes: []Note{{Path: "a", Content: "x"}, {Path: "a", Content: "x"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := validatePrepareRequest(test.request, test.notes); err == nil {
				t.Fatal("validatePrepareRequest n'a pas renvoyé d'erreur")
			}
		})
	}
}

func TestServiceRejectsOversizedAPIKey(t *testing.T) {
	t.Parallel()
	service := newService(fakePrivacy{}, &fakeCompleter{answer: "ok"}, &fakeSecretStore{})
	_, err := service.Send(context.Background(), SendRequest{
		PreviewID: strings.Repeat("p", 24),
		APIKey:    strings.Repeat("k", maxAPIKeyBytes+1),
	})
	if err != ErrInvalidRequest {
		t.Fatalf("Send error = %v, want ErrInvalidRequest", err)
	}
}

func TestServiceResolvesStoredAPIKey(t *testing.T) {
	t.Parallel()
	completion := &fakeCompleter{answer: "ok"}
	secrets := &fakeSecretStore{keys: map[Provider]string{ProviderMistral: "stored-secret"}}
	service := newService(fakePrivacy{}, completion, secrets)
	preview, err := service.Prepare(context.Background(), PrepareRequest{
		NotePaths: []string{"notes/a.md"}, Question: "Contenu ?", Provider: ProviderMistral, Model: "model",
	}, []Note{{Path: "notes/a.md", Title: "A", Content: "# A\n\nContenu utile."}})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if _, err := service.Send(context.Background(), SendRequest{PreviewID: preview.ID}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if completion.config.APIKey != "stored-secret" || secrets.getCalls != 1 {
		t.Fatalf("résolution = %q, lectures = %d", completion.config.APIKey, secrets.getCalls)
	}
}

func TestServiceRemoteKeyFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		secrets *fakeSecretStore
		want    error
	}{
		{name: "missing key", secrets: &fakeSecretStore{}, want: ErrAPIKeyNotFound},
		{name: "unavailable keyring", secrets: &fakeSecretStore{availableErr: ErrKeyringUnavailable}, want: ErrKeyringUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			completion := &fakeCompleter{answer: "must not run"}
			service := newService(fakePrivacy{}, completion, test.secrets)
			preview, err := service.Prepare(context.Background(), PrepareRequest{
				NotePaths: []string{"notes/a.md"}, Question: "Contenu ?", Provider: ProviderOpenRouter, Model: "model",
			}, []Note{{Path: "notes/a.md", Title: "A", Content: "# A\n\nContenu utile."}})
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			_, err = service.Send(context.Background(), SendRequest{PreviewID: preview.ID})
			if !errors.Is(err, test.want) {
				t.Fatalf("Send error = %v, want %v", err, test.want)
			}
			if len(completion.messages) != 0 {
				t.Fatal("le fournisseur a été appelé après l’échec de résolution")
			}
		})
	}
}

func TestServiceOllamaNeverTouchesKeyring(t *testing.T) {
	t.Parallel()
	completion := &fakeCompleter{answer: "ok"}
	secrets := &fakeSecretStore{availableErr: ErrKeyringUnavailable}
	service := newService(fakePrivacy{}, completion, secrets)
	preview, err := service.Prepare(context.Background(), PrepareRequest{
		NotePaths: []string{"notes/a.md"}, Question: "Contenu ?", Provider: ProviderOllama, Model: "local",
	}, []Note{{Path: "notes/a.md", Title: "A", Content: "# A\n\nContenu utile."}})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if _, err := service.Send(context.Background(), SendRequest{PreviewID: preview.ID, APIKey: "ignored"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if secrets.availableCalls != 0 || secrets.getCalls != 0 || completion.config.APIKey != "" {
		t.Fatalf("accès trousseau = available %d, get %d, clé %q", secrets.availableCalls, secrets.getCalls, completion.config.APIKey)
	}
}
