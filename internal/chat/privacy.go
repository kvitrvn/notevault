package chat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	goanon "github.com/bornholm/go-anon"
	goanonAnonymizer "github.com/bornholm/go-anon/pkg/anonymizer"
	"github.com/bornholm/go-anon/pkg/modelstore"
)

type privacySession interface{}

type privacyResult struct {
	Text     string
	Entities []DetectedEntity
}

type privacyEngine interface {
	NewSession() privacySession
	Anonymize(ctx context.Context, session privacySession, text string) (privacyResult, error)
	Deanonymize(session privacySession, text string) string
}

type goAnonPrivacy struct {
	mu          sync.Mutex
	store       *modelstore.Store
	detector    goanon.LanguageDetector
	anonymizers map[string]*goanonAnonymizer.Anonymizer
}

var modelStoreLogMu sync.Mutex

var goAnonPlaceholderPattern = regexp.MustCompile(`^\[([[:alpha:]][[:alnum:]_]*)_([0-9]+)\]$`)

const maxModelResponseBytes int64 = 512 << 20

type modelTransport struct {
	base http.RoundTripper
}

func (t modelTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme != "https" {
		return nil, errors.New("téléchargement de modèle non sécurisé refusé")
	}
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if response.ContentLength > maxModelResponseBytes {
		response.Body.Close()
		return nil, errors.New("modèle d’anonymisation trop volumineux")
	}
	response.Body = http.MaxBytesReader(nil, response.Body, maxModelResponseBytes)
	return response, nil
}

func newGoAnonPrivacy(cacheDir string) (*goAnonPrivacy, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	client := &http.Client{
		Timeout:   15 * time.Minute,
		Transport: modelTransport{base: transport},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("trop de redirections lors du téléchargement du modèle")
			}
			if req.URL.Scheme != "https" {
				return errors.New("redirection non sécurisée refusée")
			}
			return nil
		},
	}

	// modelstore.New écrit son chemin absolu via le logger standard. NoteVault
	// interdit les chemins personnels dans les logs : on neutralise uniquement
	// ce message de construction, sous verrou global, puis on restaure le logger.
	modelStoreLogMu.Lock()
	previousWriter := log.Writer()
	log.SetOutput(io.Discard)
	store, err := modelstore.New(
		modelstore.WithCacheDir(cacheDir),
		modelstore.WithHTTPClient(client),
	)
	log.SetOutput(previousWriter)
	modelStoreLogMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("initialiser le cache des modèles d’anonymisation : %w", err)
	}

	return &goAnonPrivacy{
		store:       store,
		detector:    goanon.NewWhatlangDetector(goanon.SupportedLanguages()...),
		anonymizers: make(map[string]*goanonAnonymizer.Anonymizer),
	}, nil
}

func (p *goAnonPrivacy) NewSession() privacySession {
	return goanonAnonymizer.NewSession()
}

func (p *goAnonPrivacy) Anonymize(ctx context.Context, session privacySession, text string) (privacyResult, error) {
	upstreamSession, ok := session.(*goanonAnonymizer.Session)
	if !ok || upstreamSession == nil {
		return privacyResult{}, errors.New("session d’anonymisation invalide")
	}
	if strings.TrimSpace(text) == "" {
		return privacyResult{Text: text, Entities: []DetectedEntity{}}, nil
	}

	language := "fr"
	if detected, err := p.detector.Detect(text); err == nil && detected.Lang != "" {
		language = detected.Lang
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	anon, err := p.anonymizer(ctx, language)
	if err != nil {
		return privacyResult{}, err
	}
	result, err := anon.Anonymize(text, goanonAnonymizer.WithSession(upstreamSession))
	if err != nil {
		return privacyResult{}, fmt.Errorf("anonymiser le texte : %w", err)
	}

	entities := make([]DetectedEntity, 0, len(result.Entities))
	for _, entity := range result.Entities {
		entities = append(entities, DetectedEntity{
			Type:        string(entity.Type),
			Placeholder: result.OriginalToPlaceholder[entity.Text],
			Confidence:  entity.Confidence,
		})
	}
	return privacyResult{Text: result.Text, Entities: entities}, nil
}

func (p *goAnonPrivacy) anonymizer(ctx context.Context, language string) (*goanonAnonymizer.Anonymizer, error) {
	if cached := p.anonymizers[language]; cached != nil {
		return cached, nil
	}
	path, err := p.store.Get(ctx, language)
	if err != nil {
		return nil, fmt.Errorf("préparer le modèle %s : %w", language, err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("ouvrir le modèle %s : %w", language, err)
	}
	defer f.Close()
	model, err := goanon.LoadModel(f)
	if err != nil {
		return nil, fmt.Errorf("charger le modèle %s : %w", language, err)
	}
	recognizer, err := goanon.NewRecognizer(
		model,
		goanon.WithLanguage(language),
		goanon.WithBuiltinRegexPatterns(),
		goanon.WithBuiltinSecretPatterns(),
	)
	if err != nil {
		return nil, fmt.Errorf("initialiser le modèle %s : %w", language, err)
	}
	anon := goanon.NewAnonymizer(recognizer, goanon.Config{
		Strategy:      goanon.TagReplace,
		ConsistentMap: true,
	})
	p.anonymizers[language] = anon
	return anon, nil
}

func (p *goAnonPrivacy) Deanonymize(session privacySession, text string) string {
	upstreamSession, ok := session.(*goanonAnonymizer.Session)
	if !ok || upstreamSession == nil || len(upstreamSession.Mapping) == 0 {
		return text
	}
	placeholders := make([]string, 0, len(upstreamSession.Mapping))
	for placeholder := range upstreamSession.Mapping {
		placeholders = append(placeholders, placeholder)
	}
	sort.Slice(placeholders, func(i, j int) bool { return len(placeholders[i]) > len(placeholders[j]) })
	for _, placeholder := range placeholders {
		original := upstreamSession.Mapping[placeholder]
		text = strings.ReplaceAll(text, placeholder, original)
		text = replacePlaceholderVariants(text, placeholder, original)
	}
	return text
}

// replacePlaceholderVariants tolerates the harmless formatting changes that
// language models sometimes apply to go-anon tokens: lower-casing, Markdown
// escaping, dropped brackets, or a space/hyphen instead of an underscore.
// Only placeholders present in the local session mapping can be restored.
func replacePlaceholderVariants(text, placeholder, original string) string {
	parts := goAnonPlaceholderPattern.FindStringSubmatch(placeholder)
	if len(parts) != 3 {
		return text
	}
	labels := strings.Split(parts[1], "_")
	escapedLabels := make([]string, 0, len(labels)+1)
	for _, label := range labels {
		escapedLabels = append(escapedLabels, regexp.QuoteMeta(label))
	}
	escapedLabels = append(escapedLabels, regexp.QuoteMeta(parts[2]))
	body := strings.Join(escapedLabels, `\\?[_ -]\s*`)
	patterns := []string{
		`(?i)\\?\[\s*` + body + `\s*\\?\]`,
		`(?i)\b` + body + `\b`,
	}
	for _, expression := range patterns {
		pattern := regexp.MustCompile(expression)
		text = pattern.ReplaceAllStringFunc(text, func(string) string { return original })
	}
	return text
}
