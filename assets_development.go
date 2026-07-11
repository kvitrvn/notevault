//go:build !production

package main

import "embed"

// Les tests et la génération des bindings n'ont pas besoin d'un build Vite.
// Wails dev utilise son serveur externe et ignore ce contenu de secours.
//
//go:embed frontend/index.html
var assets embed.FS
