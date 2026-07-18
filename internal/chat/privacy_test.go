package chat

import (
	"io"
	"net/http"
	"strings"
	"testing"

	goanonAnonymizer "github.com/bornholm/go-anon/pkg/anonymizer"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestModelTransportRejectsInsecureURL(t *testing.T) {
	t.Parallel()
	transport := modelTransport{base: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("le transport réseau ne devait pas être appelé")
		return nil, nil
	})}
	request, err := http.NewRequest(http.MethodGet, "http://example.test/model", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(request); err == nil {
		t.Fatal("le téléchargement HTTP a été accepté")
	}
}

func TestModelTransportLimitsResponseSize(t *testing.T) {
	t.Parallel()
	transport := modelTransport{base: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Body:          io.NopCloser(strings.NewReader("model")),
			ContentLength: maxModelResponseBytes + 1,
			Request:       request,
		}, nil
	})}
	request, err := http.NewRequest(http.MethodGet, "https://example.test/model", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(request); err == nil {
		t.Fatal("la réponse surdimensionnée a été acceptée")
	}
}

func TestDeanonymizeRestoresLLMPlaceholderVariants(t *testing.T) {
	t.Parallel()
	session := goanonAnonymizer.NewSession()
	session.Mapping["[PERSON_1]"] = "Nadia Ferreira"
	session.Mapping["[IP_ADDRESS_2]"] = "192.0.2.42"
	privacy := &goAnonPrivacy{}

	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "exact", text: "[PERSON_1]", want: "Nadia Ferreira"},
		{name: "lower case", text: "[person_1]", want: "Nadia Ferreira"},
		{name: "markdown escaped", text: `\[PERSON\_1\]`, want: "Nadia Ferreira"},
		{name: "without brackets", text: "PERSON_1", want: "Nadia Ferreira"},
		{name: "spaces", text: "[PERSON 1]", want: "Nadia Ferreira"},
		{name: "compound label", text: "IP-ADDRESS 2", want: "192.0.2.42"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := privacy.Deanonymize(session, test.text); got != test.want {
				t.Fatalf("Deanonymize(%q) = %q, want %q", test.text, got, test.want)
			}
		})
	}
}
