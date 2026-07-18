package chat

import (
	"net/http"
	"testing"
)

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

func TestProviderEndpointRejectsUnknownProvider(t *testing.T) {
	t.Parallel()
	if _, _, err := providerEndpoint("custom", &http.Client{}, &http.Client{}); err == nil {
		t.Fatal("providerEndpoint(custom) n'a pas renvoyé d'erreur")
	}
}
