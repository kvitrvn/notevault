package vault

import (
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// AssetServer expose les fichiers du coffre (images, pièces jointes) sur un
// port HTTP local. Nécessaire parce que les `<img src=...>` injectés par
// Tiptap ne peuvent pas charger un chemin relatif sur le système de fichiers
// — la webview a besoin d'une URL http://.
//
// Le port est attribué dynamiquement par le kernel (port 0), ce qui évite
// les collisions. L'URL absolue est reconstruite par l'App pour chaque
// chemin relatif via AssetURL().
//
// Toutes les requêtes sont confinées à `<root>/assets/` avec une whitelist
// d'extensions (réutilise sanitizeExt) et un check anti-traversal.
type AssetServer struct {
	assetsDir string
	listener  net.Listener
	server    *http.Server
	mu        sync.Mutex
	running   bool
}

func NewAssetServer(root string) *AssetServer {
	return &AssetServer{assetsDir: filepath.Join(root, "assets")}
}

// Start démarre le serveur HTTP sur 127.0.0.1:<port libre>. Retourne le port.
func (s *AssetServer) Start() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return 0, fmt.Errorf("serveur d'assets déjà démarré")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/files/", s.handleFiles)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("listen asset server : %w", err)
	}
	s.listener = listener
	s.server = &http.Server{Handler: mux}
	s.running = true

	port := listener.Addr().(*net.TCPAddr).Port
	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("asset server: %v", err)
		}
	}()
	return port, nil
}

// Stop ferme le serveur HTTP. Idempotent.
func (s *AssetServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return nil
	}
	s.running = false
	return s.server.Close()
}

// Port retourne le port sur lequel le serveur écoute. 0 si non démarré.
func (s *AssetServer) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return 0
	}
	return s.listener.Addr().(*net.TCPAddr).Port
}

func (s *AssetServer) handleFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rel := strings.TrimPrefix(r.URL.Path, "/files/")
	assetPath, err := normalizeAssetPath(rel)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if assetPath == "" {
		http.NotFound(w, r)
		return
	}

	// Whitelist d'extension avant tout (sécurité : bloque .exe, .html, etc.).
	if sanitizeExt(assetPath) == "" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// os.Root confine aussi l'ouverture face aux liens symboliques qui
	// pointeraient hors de <vault>/assets.
	root, err := os.OpenRoot(s.assetsDir)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer root.Close()
	file, err := root.Open(assetPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		// os.Root renvoie notamment une erreur ici lorsqu'un symlink tente de
		// sortir de la racine autorisée. Ne pas révéler plus de détails.
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if stat.IsDir() {
		http.NotFound(w, r)
		return
	}

	ext := filepath.Ext(assetPath)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filepath.ToSlash(assetPath), stat.ModTime(), file)
}
