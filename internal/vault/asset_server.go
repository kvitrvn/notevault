package vault

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
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
// chemin relatif via AssetURL(), et embarque un token de session généré
// à chaque démarrage pour éviter qu'un autre process local ne lise les
// assets en découvrant le port.
//
// Toutes les requêtes sont confinées à `<root>/assets/` avec une whitelist
// d'extensions (réutilise sanitizeExt) et un check anti-traversal.
type AssetServer struct {
	assetsDir string
	token     string
	listener  net.Listener
	server    *http.Server
	mu        sync.Mutex
	running   bool
}

func NewAssetServer(root string) *AssetServer {
	return &AssetServer{assetsDir: filepath.Join(root, "assets")}
}

// Token retourne le token de session courant. Chaîne vide si le serveur
// n'est pas démarré.
func (s *AssetServer) Token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

// Start démarre le serveur HTTP sur 127.0.0.1:<port libre>. Retourne le port.
func (s *AssetServer) Start() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return 0, fmt.Errorf("serveur d'assets déjà démarré")
	}

	token, err := newSessionToken()
	if err != nil {
		return 0, fmt.Errorf("générer le token de session : %w", err)
	}
	s.token = token

	mux := http.NewServeMux()
	mux.HandleFunc("/files/", s.handleFiles)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		s.token = ""
		return 0, fmt.Errorf("listen asset server : %w", err)
	}
	s.listener = listener
	server := &http.Server{Handler: mux}
	s.server = server
	s.running = true

	port := listener.Addr().(*net.TCPAddr).Port
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
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
	server := s.server
	s.server = nil
	s.listener = nil
	s.token = ""
	return server.Close()
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

// newSessionToken génère un token de session aléatoire (32 octets en
// hexadécimal = 64 caractères). Utilisé pour authentifier les requêtes
// d'asset : toute requête sans token valide est rejetée en 403.
func newSessionToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// checkToken compare le token fourni au token de session en temps constant
// pour bloquer les attaques par analyse de timing. Retourne false si le
// serveur n'est pas démarré ou si le token est manquant / incorrect.
func (s *AssetServer) checkToken(provided string) bool {
	s.mu.Lock()
	expected := s.token
	s.mu.Unlock()
	if expected == "" || provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func (s *AssetServer) handleFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Authentification : le token est passé en query (?t=...) car les
	// `<img src>` standards n'envoient pas d'en-têtes personnalisés.
	if !s.checkToken(r.URL.Query().Get("t")) {
		http.Error(w, "forbidden", http.StatusForbidden)
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
	// SVG : un fichier SVG peut contenir du script s'il est chargé en
	// document top-level (`image/svg+xml` autorise script dans ce contexte).
	// On bloque explicitement l'exécution de script via CSP. Combiné à
	// `nosniff`, cela ferme la porte à l'exfiltration pilotée par un SVG
	// malveillant, tout en gardant le rendu inline via `<img>`.
	if strings.EqualFold(ext, ".svg") {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	}
	http.ServeContent(w, r, filepath.ToSlash(assetPath), stat.ModTime(), file)
}
