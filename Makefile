WAILS_VERSION ?= v2.12.0
WAILS := tools/wails/bin/wails

.PHONY: dev regen build test fmt check patch-models

# Workflow dev : `wails dev` régénère les bindings à chaque démarrage,
# ce qui écrase la classe Time corrigée. Le hook `frontend:dev:watcher`
# dans wails.json (scripts/with-patch-dev.sh) ré-applique le patch juste
# avant de lancer Vite, donc les éditions TypeScript voient la bonne classe.
dev: $(WAILS)
	$(WAILS) dev

# À utiliser après avoir modifié du code Go exposé : régénère les
# bindings puis applique le patch.
regen: $(WAILS)
	$(WAILS) generate module
	./scripts/patch-models.sh

build: $(WAILS)
	$(WAILS) build

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

$(WAILS):
	mkdir -p tools/wails/bin
	GOBIN=$(PWD)/tools/wails/bin go install github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_VERSION)
