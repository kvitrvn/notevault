package chat

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestProviderEndpointUsesFixedDestinations(t *testing.T) {
	t.Parallel()
	local := &http.Client{}
	remote := &http.Client{}
	tests := []struct {
		name     string
		provider Provider
		wantURL  string
		want     *http.Client
	}{
		{name: "ollama loopback", provider: ProviderOllama, wantURL: "http://127.0.0.1:11434/v1/chat/completions", want: local},
		{name: "openai", provider: ProviderOpenAI, wantURL: "https://api.openai.com/v1/chat/completions", want: remote},
		{name: "mistral", provider: ProviderMistral, wantURL: "https://api.mistral.ai/v1/chat/completions", want: remote},
		{name: "openrouter", provider: ProviderOpenRouter, wantURL: "https://openrouter.ai/api/v1/chat/completions", want: remote},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			gotURL, gotClient, err := providerEndpoint(test.provider, local, remote)
			if err != nil {
				t.Fatalf("providerEndpoint: %v", err)
			}
			if gotURL != test.wantURL || gotClient != test.want {
				t.Fatalf("got (%q, %p), want (%q, %p)", gotURL, gotClient, test.wantURL, test.want)
			}
		})
	}
}

func TestProviderClientKeepsAPIKeyOutOfPayload(t *testing.T) {
	t.Parallel()
	const apiKey = "header-only-secret"
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if strings.Contains(string(body), apiKey) {
			t.Fatal("la clé API apparaît dans le payload JSON")
		}
		if request.Header.Get("Authorization") != "Bearer "+apiKey {
			t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"ok"}}]}`)),
			Header:     make(http.Header),
		}, nil
	})}
	provider := &providerClient{local: client, remote: client}
	answer, err := provider.Complete(context.Background(), completionConfig{
		Provider: ProviderOpenAI,
		Model:    "model",
		APIKey:   apiKey,
	}, []safeMessage{{Role: "user", Content: "question"}})
	if err != nil || answer != "ok" {
		t.Fatalf("Complete = %q, %v", answer, err)
	}
}

func TestProviderEndpointRejectsUnknownProvider(t *testing.T) {
	t.Parallel()
	if _, _, err := providerEndpoint("custom", &http.Client{}, &http.Client{}); err == nil {
		t.Fatal("providerEndpoint(custom) n'a pas renvoyé d'erreur")
	}
}
