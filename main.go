package main

import (
	"fmt"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var buildVersion = "dev"

func main() {
	if isInternalPDFWorker(os.Args) {
		if err := runInternalPDFWorker(os.Args[2:], os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(buildVersion)
		return
	}
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

// singleInstanceID identifie le verrou d'instance unique. Deux processus
// NoteVault sur la même machine s'écraseraient mutuellement sur la config
// applicative (dernier écrivain gagne), feraient tourner deux watchers sur le
// même coffre, et pourraient lire pendant une migration de chiffrement.
const singleInstanceID = "dev.kvitrvn.notevault"

func applicationOptions(app *App) *options.App {
	return &options.App{
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: singleInstanceID,
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				// Le second processus s'arrête ; on ramène la fenêtre
				// existante au premier plan plutôt que de l'ignorer.
				if app.ctx != nil {
					wailsruntime.WindowUnminimise(app.ctx)
					wailsruntime.WindowShow(app.ctx)
				}
			},
		},
		Title:         "NoteVault",
		Width:         1280,
		Height:        820,
		MinWidth:      960,
		MinHeight:     640,
		Frameless:     true,
		DisableResize: false,
		Fullscreen:    false,
		AssetServer: &assetserver.Options{
			Assets:     assets,
			Middleware: securityHeadersMiddleware,
		},
		// Wails active ce menu en développement mais le désactive par défaut
		// en production. Il est nécessaire aux actions natives de l'éditeur
		// (couper, copier, coller et sélection).
		EnableDefaultContextMenu: true,
		Linux: &linux.Options{
			Icon:             applicationIcon,
			ProgramName:      "notevault",
			WebviewGpuPolicy: linux.WebviewGpuPolicyNever,
		},
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
