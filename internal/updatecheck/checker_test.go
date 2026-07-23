package updatecheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckerComparesStrictVersions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		current   string
		latest    string
		available bool
	}{
		{name: "newer patch", current: "v0.3.2", latest: "v0.3.3", available: true},
		{name: "newer major", current: "v2.12.8", latest: "v10.0.0", available: true},
		{name: "same", current: "v0.3.2", latest: "v0.3.2"},
		{name: "older", current: "v1.4.0", latest: "v1.3.9"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
					t.Errorf("Accept = %q", got)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"tag_name":"` + test.latest + `"}`))
			}))
			defer server.Close()

			result, err := New(server.Client(), server.URL).Check(t.Context(), test.current)
			if err != nil {
				t.Fatal(err)
			}
			if result.CurrentVersion != test.current ||
				result.LatestVersion != test.latest ||
				result.UpdateAvailable != test.available {
				t.Fatalf("result = %+v", result)
			}
		})
	}
}

func TestCheckerUsesFiveSecondDefaultTimeout(t *testing.T) {
	t.Parallel()
	checker := New(nil, "")
	if checker.client.Timeout != 5*time.Second {
		t.Fatalf("timeout = %s, want 5s", checker.client.Timeout)
	}
	if checker.endpoint != LatestReleaseEndpoint {
		t.Fatalf("endpoint = %q", checker.endpoint)
	}
}

func TestCheckerRejectsInvalidResponses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "invalid tag", status: http.StatusOK, body: `{"tag_name":"v1.2"}`},
		{name: "prerelease tag", status: http.StatusOK, body: `{"tag_name":"v1.2.3-beta.1"}`},
		{name: "leading zero", status: http.StatusOK, body: `{"tag_name":"v01.2.3"}`},
		{name: "malformed JSON", status: http.StatusOK, body: `{"tag_name":`},
		{name: "trailing JSON", status: http.StatusOK, body: `{"tag_name":"v1.2.3"} {}`},
		{name: "non 200", status: http.StatusNotFound, body: `{}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			if _, err := New(server.Client(), server.URL).Check(t.Context(), "v1.0.0"); err == nil {
				t.Fatal("Check() a accepté une réponse invalide")
			}
		})
	}
}

func TestCheckerRejectsInvalidCurrentVersionWithoutRequest(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	defer server.Close()

	if _, err := New(server.Client(), server.URL).Check(t.Context(), "1.0.0"); err == nil {
		t.Fatal("Check() a accepté une version courante invalide")
	}
	if requests.Load() != 0 {
		t.Fatalf("requêtes = %d, want 0", requests.Load())
	}
}

func TestCheckerDevVersionDoesNotMakeRequest(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	result, err := New(server.Client(), server.URL).Check(t.Context(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if result != (Result{CurrentVersion: "dev"}) {
		t.Fatalf("result = %+v", result)
	}
	if requests.Load() != 0 {
		t.Fatalf("requêtes = %d, want 0", requests.Load())
	}
}

func TestCheckerHonoursClientTimeout(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	defer server.Close()

	client := &http.Client{Timeout: 10 * time.Millisecond}
	_, err := New(client, server.URL).Check(t.Context(), "v1.0.0")
	if err == nil {
		t.Fatal("Check() n’a pas expiré")
	}
}

func TestCheckerHonoursCancellation(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := New(server.Client(), server.URL).Check(ctx, "v1.0.0")
		done <- err
	}()
	<-started
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Check() n’a pas respecté l’annulation")
	}
}
