package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveThrough(t *testing.T, path string) *http.Response {
	t.Helper()
	handler := securityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder.Result()
}

func TestSecurityHeadersOnDocuments(t *testing.T) {
	for _, path := range []string{"/", "/index.html"} {
		t.Run(path, func(t *testing.T) {
			response := serveThrough(t, path)
			if got := response.Header.Get("Content-Security-Policy"); got != contentSecurityPolicy {
				t.Fatalf("CSP = %q, want %q", got, contentSecurityPolicy)
			}
			if got := response.Header.Get("Referrer-Policy"); got != "no-referrer" {
				t.Fatalf("Referrer-Policy = %q", got)
			}
		})
	}
}

func TestSecurityHeadersNosniffEverywhere(t *testing.T) {
	response := serveThrough(t, "/assets/index-abc.js")
	if got := response.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := response.Header.Get("Content-Security-Policy"); got != "" {
		t.Fatalf("CSP posée sur un sous-fichier : %q", got)
	}
}

// Les directives qui empêchent une exfiltration depuis la webview ne doivent
// jamais autoriser d'hôte distant.
func TestContentSecurityPolicyBlocksRemoteEgress(t *testing.T) {
	directives := map[string]string{}
	for _, raw := range strings.Split(contentSecurityPolicy, ";") {
		fields := strings.Fields(strings.TrimSpace(raw))
		if len(fields) == 0 {
			continue
		}
		directives[fields[0]] = strings.Join(fields[1:], " ")
	}

	if got, ok := directives["default-src"]; !ok || got != "'none'" {
		t.Fatalf("default-src = %q, want 'none'", got)
	}
	for _, name := range []string{"connect-src", "img-src", "media-src", "script-src", "font-src"} {
		value, ok := directives[name]
		if !ok {
			t.Fatalf("directive %s absente", name)
		}
		for _, token := range strings.Fields(value) {
			if strings.HasPrefix(token, "https://") || strings.HasPrefix(token, "http://") {
				if token == "http://127.0.0.1:*" {
					continue // asset server local, port attribué par le noyau
				}
				t.Fatalf("%s autorise un hôte distant : %q", name, token)
			}
			if token == "*" || token == "'unsafe-eval'" {
				t.Fatalf("%s trop permissif : %q", name, token)
			}
		}
	}
	if got := directives["script-src"]; got != "'self'" {
		t.Fatalf("script-src = %q, want 'self' (le bundle Vite n'a aucun script inline)", got)
	}
	for _, name := range []string{"object-src", "base-uri", "form-action", "frame-ancestors"} {
		if got := directives[name]; got != "'none'" {
			t.Fatalf("%s = %q, want 'none'", name, got)
		}
	}
}
