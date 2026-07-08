package vault

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newAssetServerTB(t *testing.T) (*AssetServer, string, func()) {
	t.Helper()
	root := t.TempDir()
	srv := NewAssetServer(root)
	if _, err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	cleanup := func() { _ = srv.Stop() }
	return srv, root, cleanup
}

func writeAsset(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(abs, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestAssetServerServesAllowedFile(t *testing.T) {
	srv, root, stop := newAssetServerTB(t)
	defer stop()
	payload := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	writeAsset(t, root, "assets/2026/07/uuid.png", payload)

	url := "http://127.0.0.1:" + portToString(srv.Port()) + "/files/assets/2026/07/uuid.png"
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/png") {
		t.Fatalf("content-type: %q", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc == "" {
		t.Fatalf("cache-control vide")
	}
	got, _ := io.ReadAll(resp.Body)
	if string(got) != string(payload) {
		t.Fatalf("payload mismatch")
	}
}

func TestAssetServerBlocksTraversal(t *testing.T) {
	srv, _, stop := newAssetServerTB(t)
	defer stop()

	cases := []string{
		"/files/../etc/passwd",
		"/files/assets/../../etc/passwd",
		"/files/assets/../foo.png",
	}
	for _, p := range cases {
		t.Run(strings.TrimPrefix(p, "/files/"), func(t *testing.T) {
			resp, err := http.Get("http://127.0.0.1:" + portToString(srv.Port()) + p)
			if err != nil {
				t.Fatalf("GET %s: %v", p, err)
			}
			resp.Body.Close()
			if resp.StatusCode != 403 && resp.StatusCode != 404 {
				t.Fatalf("%s : status %d attendu 403/404", p, resp.StatusCode)
			}
		})
	}
}

func TestAssetServerBlocksBadExtension(t *testing.T) {
	srv, root, stop := newAssetServerTB(t)
	defer stop()
	writeAsset(t, root, "assets/2026/07/payload.exe", []byte("MZ"))

	url := "http://127.0.0.1:" + portToString(srv.Port()) + "/files/assets/2026/07/payload.exe"
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("status %d, attendu 403", resp.StatusCode)
	}
}

func TestAssetServerNotFound(t *testing.T) {
	srv, _, stop := newAssetServerTB(t)
	defer stop()
	url := "http://127.0.0.1:" + portToString(srv.Port()) + "/files/assets/2026/07/missing.png"
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("status %d, attendu 404", resp.StatusCode)
	}
}

func TestAssetServerRootListingIsForbidden(t *testing.T) {
	srv, root, stop := newAssetServerTB(t)
	defer stop()
	writeAsset(t, root, "assets/dir.png", []byte("data"))

	// /files/ (sans chemin) -> 404 ou 403, jamais la liste.
	resp, err := http.Get("http://127.0.0.1:" + portToString(srv.Port()) + "/files/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Fatalf("listing racine不应该获得 200")
	}
}

func TestAssetServerMIMEPerExtension(t *testing.T) {
	srv, root, stop := newAssetServerTB(t)
	defer stop()
	cases := map[string]string{
		"assets/a.svg":  "image/svg",
		"assets/b.jpg":  "image/jpeg",
		"assets/c.webp": "image/webp",
		"assets/d.md":   "text/",
	}
	for rel, prefix := range cases {
		writeAsset(t, root, rel, []byte("data"))
		url := "http://127.0.0.1:" + portToString(srv.Port()) + "/files/" + rel
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("GET %s: %v", rel, err)
		}
		ct := resp.Header.Get("Content-Type")
		resp.Body.Close()
		if !strings.HasPrefix(ct, prefix) {
			t.Errorf("%s: content-type %q ne commence pas par %q", rel, ct, prefix)
		}
	}
}

func TestAssetServerIdempotentStop(t *testing.T) {
	srv, _, _ := newAssetServerTB(t)
	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop #1: %v", err)
	}
	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop #2: %v", err)
	}
}

// portToString évite strconv pour alléger les imports.
func portToString(p int) string {
	if p == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for p > 0 {
		i--
		b[i] = byte('0' + p%10)
		p /= 10
	}
	return string(b[i:])
}
