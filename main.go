package main

import (
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

func main() {
	if err := configureDisplayBackend(); err != nil {
		log.Fatal(err)
	}
	app, err := newApplication()
	if err != nil {
		log.Fatal(err)
	}

	err = wails.Run(applicationOptions(app))
	if err != nil {
		log.Fatal(err)
	}
}

func applicationOptions(app *App) *options.App {
	return &options.App{
		Title:         "NoteVault",
		Width:         1280,
		Height:        820,
		MinWidth:      960,
		MinHeight:     640,
		Frameless:     true,
		DisableResize: false,
		Fullscreen:    false,
		AssetServer:   &assetserver.Options{Assets: assets},
		// NewRGB garantit un fond natif opaque (alpha 255).
		BackgroundColour: options.NewRGB(24, 24, 27),
		Debug: options.Debug{
			OpenInspectorOnStartup: os.Getenv("NOTEAULT_INSPECTOR") == "1",
		},
		OnStartup:  app.Startup,
		OnShutdown: app.Shutdown,
		Bind: []interface{}{
			app,
		},
	}
}
