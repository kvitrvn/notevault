package vault

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func newAssetServerTB(t *testing.T) (*AssetServer, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	return NewAssetServer(root), root
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

func assetRequest(t *testing.T, srv *AssetServer, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	response := httptest.NewRecorder()
	srv.handleFiles(response, req)
	return response
}

func TestAssetServerServesAllowedFile(t *testing.T) {
	srv, root := newAssetServerTB(t)
	payload := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	writeAsset(t, root, "assets/2026/07/uuid.png", payload)

	response := assetRequest(t, srv, http.MethodGet, "/files/assets/2026/07/uuid.png")
	if response.Code != http.StatusOK {
		t.Fatalf("status: %d", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "image/png") {
		t.Fatalf("content-type: %q", contentType)
	}
	if response.Header().Get("Cache-Control") == "" {
		t.Fatal("cache-control vide")
	}
	got, err := io.ReadAll(response.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatal("payload mismatch")
	}
}

func TestAssetServerBlocksTraversal(t *testing.T) {
	srv, _ := newAssetServerTB(t)
	tests := []struct {
		name string
		path string
	}{
		{name: "parent before assets", path: "/files/../etc/passwd"},
		{name: "escape assets", path: "/files/assets/../../etc/passwd"},
		{name: "leave assets", path: "/files/assets/../foo.png"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := assetRequest(t, srv, http.MethodGet, tt.path)
			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
			}
		})
	}
}

func TestAssetServerBlocksFilesOutsideAssets(t *testing.T) {
	srv, root := newAssetServerTB(t)
	writeAsset(t, root, "notes/secret.png", []byte("not an asset"))

	response := assetRequest(t, srv, http.MethodGet, "/files/notes/secret.png")
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestAssetServerBlocksSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("la création de symlinks nécessite souvent des privilèges sous Windows")
	}
	srv, root := newAssetServerTB(t)
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}
	assetsDir := filepath.Join(root, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(assetsDir, "escape.png")); err != nil {
		t.Fatal(err)
	}

	response := assetRequest(t, srv, http.MethodGet, "/files/assets/escape.png")
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestAssetServerBlocksBadExtension(t *testing.T) {
	srv, root := newAssetServerTB(t)
	writeAsset(t, root, "assets/2026/07/payload.exe", []byte("MZ"))

	response := assetRequest(t, srv, http.MethodGet, "/files/assets/2026/07/payload.exe")
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestAssetServerNotFound(t *testing.T) {
	srv, _ := newAssetServerTB(t)
	response := assetRequest(t, srv, http.MethodGet, "/files/assets/2026/07/missing.png")
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestAssetServerRootListingIsForbidden(t *testing.T) {
	srv, root := newAssetServerTB(t)
	writeAsset(t, root, "assets/dir.png", []byte("data"))

	response := assetRequest(t, srv, http.MethodGet, "/files/")
	if response.Code == http.StatusOK {
		t.Fatal("la racine ne doit jamais être listée")
	}
}

func TestAssetServerMIMEPerExtension(t *testing.T) {
	srv, root := newAssetServerTB(t)
	tests := []struct {
		path       string
		wantPrefix string
	}{
		{path: "assets/a.svg", wantPrefix: "image/svg"},
		{path: "assets/b.jpg", wantPrefix: "image/jpeg"},
		{path: "assets/c.webp", wantPrefix: "image/webp"},
		{path: "assets/d.md", wantPrefix: "text/"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			writeAsset(t, root, tt.path, []byte("data"))
			response := assetRequest(t, srv, http.MethodGet, "/files/"+tt.path)
			contentType := response.Header().Get("Content-Type")
			if !strings.HasPrefix(contentType, tt.wantPrefix) {
				t.Fatalf("content-type %q does not start with %q", contentType, tt.wantPrefix)
			}
		})
	}
}

func TestAssetServerRejectsUnsupportedMethods(t *testing.T) {
	srv, _ := newAssetServerTB(t)
	response := assetRequest(t, srv, http.MethodPost, "/files/assets/a.png")
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func TestAssetServerIdempotentStop(t *testing.T) {
	srv, _ := newAssetServerTB(t)
	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop #1: %v", err)
	}
	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop #2: %v", err)
	}
}
