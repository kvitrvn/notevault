package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxProviderResponseBytes = 4 << 20

type safeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type completionConfig struct {
	Provider Provider
	Model    string
	APIKey   string
}

type completer interface {
	Complete(ctx context.Context, config completionConfig, messages []safeMessage) (string, error)
}

type providerClient struct {
	local  *http.Client
	remote *http.Client
}

func newProviderClient() *providerClient {
	localTransport := http.DefaultTransport.(*http.Transport).Clone()
	localTransport.Proxy = nil
	remoteTransport := http.DefaultTransport.(*http.Transport).Clone()
	remoteTransport.Proxy = http.ProxyFromEnvironment
	return &providerClient{
		local: &http.Client{
			Timeout:   90 * time.Second,
			Transport: localTransport,
		},
		remote: &http.Client{
			Timeout:   90 * time.Second,
			Transport: remoteTransport,
		},
	}
}

func (c *providerClient) Complete(ctx context.Context, config completionConfig, messages []safeMessage) (string, error) {
	endpoint, client, err := providerEndpoint(config.Provider, c.local, c.remote)
	if err != nil {
		return "", err
	}
	model := strings.TrimSpace(config.Model)
	if model == "" {
		return "", fmt.Errorf("%w : modèle manquant", ErrInvalidRequest)
	}
	if config.Provider != ProviderOllama && strings.TrimSpace(config.APIKey) == "" {
		return "", fmt.Errorf("%w : clé API manquante", ErrInvalidRequest)
	}

	body, err := json.Marshal(struct {
		Model    string        `json:"model"`
		Messages []safeMessage `json:"messages"`
		Stream   bool          `json:"stream"`
	}{Model: model, Messages: messages, Stream: false})
	if err != nil {
		return "", fmt.Errorf("encoder la requête au modèle : %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("préparer la requête au modèle : %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "NoteVault/0.1")
	if config.Provider != ProviderOllama {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(config.APIKey))
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("contacter le fournisseur %s : %w", config.Provider, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return "", fmt.Errorf("le fournisseur %s a refusé la requête (%s)", config.Provider, resp.Status)
	}

	limited := io.LimitReader(resp.Body, maxProviderResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("lire la réponse du fournisseur : %w", err)
	}
	if len(data) > maxProviderResponseBytes {
		return "", errors.New("réponse du fournisseur trop volumineuse")
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("décoder la réponse du fournisseur : %w", err)
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return "", errors.New("le fournisseur n’a renvoyé aucune réponse")
	}
	return decoded.Choices[0].Message.Content, nil
}

func providerEndpoint(provider Provider, local, remote *http.Client) (string, *http.Client, error) {
	switch provider {
	case ProviderOllama:
		return "http://127.0.0.1:11434/v1/chat/completions", local, nil
	case ProviderOpenAI:
		return "https://api.openai.com/v1/chat/completions", remote, nil
	case ProviderMistral:
		return "https://api.mistral.ai/v1/chat/completions", remote, nil
	case ProviderOpenRouter:
		return "https://openrouter.ai/api/v1/chat/completions", remote, nil
	default:
		return "", nil, fmt.Errorf("%w : fournisseur inconnu", ErrInvalidRequest)
	}
}
