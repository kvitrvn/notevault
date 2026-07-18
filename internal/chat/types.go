package chat

import "errors"

const (
	maxSelectedNotes = 50
	maxQuestionRunes = 4_000
	maxModelRunes    = 120
	maxNoteBytes     = 1 << 20
	maxTotalBytes    = 5 << 20
	maxExcerpts      = 5
	maxExcerptWords  = 120
	maxAPIKeyBytes   = 16 << 10
)

var (
	ErrEncryptedVault     = errors.New("le chat n’est pas disponible pour un coffre chiffré")
	ErrInvalidRequest     = errors.New("requête de chat invalide")
	ErrPreviewExpired     = errors.New("l’aperçu a expiré ; préparez de nouveau la question")
	ErrConversationBusy   = errors.New("une réponse est déjà en cours pour cette conversation")
	ErrConversationAbsent = errors.New("conversation introuvable")
)

type Provider string

const (
	ProviderOllama     Provider = "ollama"
	ProviderOpenAI     Provider = "openai"
	ProviderMistral    Provider = "mistral"
	ProviderOpenRouter Provider = "openrouter"
)

type PrepareRequest struct {
	ConversationID string   `json:"conversationID"`
	NotePaths      []string `json:"notePaths"`
	Question       string   `json:"question"`
	Provider       Provider `json:"provider"`
	Model          string   `json:"model"`
}

type SendRequest struct {
	PreviewID string `json:"previewID"`
	APIKey    string `json:"apiKey"`
}

type DetectedEntity struct {
	Type        string  `json:"type"`
	Placeholder string  `json:"placeholder"`
	Confidence  float64 `json:"confidence"`
}

type PreviewExcerpt struct {
	SourceID   string           `json:"sourceID"`
	Path       string           `json:"path"`
	Title      string           `json:"title"`
	Section    string           `json:"section"`
	Original   string           `json:"original"`
	Anonymized string           `json:"anonymized"`
	Entities   []DetectedEntity `json:"entities"`
}

type Preview struct {
	ID                 string           `json:"id"`
	ConversationID     string           `json:"conversationID"`
	Provider           Provider         `json:"provider"`
	Model              string           `json:"model"`
	AnonymizedQuestion string           `json:"anonymizedQuestion"`
	OutboundText       string           `json:"outboundText"`
	Excerpts           []PreviewExcerpt `json:"excerpts"`
	ExpiresAt          string           `json:"expiresAt"`
}

type Citation struct {
	SourceID string `json:"sourceID"`
	Path     string `json:"path"`
	Title    string `json:"title"`
	Section  string `json:"section"`
}

type Response struct {
	ConversationID string     `json:"conversationID"`
	Answer         string     `json:"answer"`
	Citations      []Citation `json:"citations"`
}

type Note struct {
	Path    string
	Title   string
	Content string
}
