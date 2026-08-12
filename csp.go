package main

import (
	"net/http"
	"strings"
)

// contentSecurityPolicy est la politique appliquée à la fenêtre principale.
//
// C'est la seule surface qui rend du Markdown utilisateur : le rendu passe par
// les schémas de nœuds ProseMirror, qui échappent le HTML brut, mais la CSP
// reste la défense en profondeur qui manquait — les deux surfaces dérivées
// (SVG servis par l'asset server, HTML d'export PDF) en avaient déjà une.
//
// Les axes qui comptent pour l'exfiltration sont fermés : ni `connect-src`, ni
// `img-src`, ni `media-src` n'autorisent d'hôte distant, donc aucune requête
// sortante ne peut partir de la webview. `script-src 'self'` refuse tout script
// injecté ; le bundle Vite ne produit aucun script inline (voir dist/index.html).
//
// Deux permissivités assumées :
//   - `style-src 'unsafe-inline'` : prosemirror-view injecte un <style> à
//     l'exécution, et les thèmes du coffre écrivent des propriétés CSS
//     personnalisées (valeurs déjà validées par internal/vault/themes.go).
//   - `img-src`/`media-src http://127.0.0.1:*` : l'asset server local écoute sur
//     un port attribué par le noyau, inconnu à la compilation. Il exige déjà un
//     jeton de session et reste confiné à assets/.
const contentSecurityPolicy = "default-src 'none'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: http://127.0.0.1:*; " +
	"media-src 'self' http://127.0.0.1:*; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"base-uri 'none'; " +
	"form-action 'none'; " +
	"frame-ancestors 'none'; " +
	"object-src 'none'"

// securityHeadersMiddleware pose la CSP sur les réponses du serveur d'assets
// interne de Wails. On l'applique ici plutôt que dans une balise <meta> de
// index.html pour qu'elle couvre toutes les réponses et ne puisse pas être
// contournée en remplaçant le document.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Seuls les documents portent utilement la politique ; l'ajouter aux
		// scripts et feuilles de style n'a aucun effet et alourdit les réponses.
		if isDocumentRequest(r.URL.Path) {
			w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
			w.Header().Set("Referrer-Policy", "no-referrer")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func isDocumentRequest(path string) bool {
	if path == "" || strings.HasSuffix(path, "/") {
		return true
	}
	return strings.HasSuffix(path, ".html")
}
