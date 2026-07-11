//go:build production

package main

import "embed"

// Le build Wails ajoute le tag production après avoir compilé le frontend.
//
//go:embed all:frontend/dist
var assets embed.FS
