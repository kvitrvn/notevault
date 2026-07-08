.PHONY: dev regen build test fmt check patch-models

# Workflow dev : `wails dev` régénère les bindings à chaque démarrage,
# ce qui écrase la classe Time corrigée. Le hook `frontend:dev:watcher`
# dans wails.json (scripts/with-patch-dev.sh) ré-applique le patch juste
# avant de lancer Vite, donc les éditions TypeScript voient la bonne classe.
dev:
	wails dev

# À utiliser après avoir modifié du code Go exposé : régénère les
# bindings puis applique le patch.
regen:
	wails generate module
	./scripts/patch-models.sh

build:
	wails build

test: patch-models
	go test ./...

fmt:
	gofmt -w .

check: patch-models
	cd frontend && npm run check

# Patch le fichier wailsjs/go/models.ts généré. Le générateur Wails
# produit une classe `Time` vide qui ne préserve pas les dates : Go reçoit
# `{}` et refuse de désérialiser. Ce script ajoute un toJSON() qui
# convertit correctement les Date en string ISO. Lancé automatiquement
# par les hooks dans wails.json.
patch-models:
	./scripts/patch-models.sh
