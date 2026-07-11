WAILS_VERSION ?= v2.12.0
WAILS := tools/wails/bin/wails
# Arch/Omarchy et les distributions Linux récentes fournissent WebKitGTK 4.1.
# Wails utilise encore 4.0 par défaut, sauf avec ce tag officiel.
WAILS_TAGS ?= $(shell pkg-config --exists webkit2gtk-4.1 2>/dev/null && echo webkit2_41)
WAILS_TAG_ARGS := $(if $(WAILS_TAGS),-tags $(WAILS_TAGS),)

.PHONY: dev regen build test frontend-test fmt check verify patch-models frontend-install

# Workflow dev : `wails dev` régénère les bindings à chaque démarrage,
# ce qui écrase la classe Time corrigée. Le hook `frontend:dev:watcher`
# dans wails.json (scripts/with-patch-dev.sh) ré-applique le patch juste
# avant de lancer Vite, donc les éditions TypeScript voient la bonne classe.
dev: $(WAILS)
	$(WAILS) dev $(WAILS_TAG_ARGS)

# À utiliser après avoir modifié du code Go exposé : régénère les
# bindings puis applique le patch.
regen: $(WAILS)
	$(WAILS) generate module
	./scripts/patch-models.sh

build: $(WAILS)
	$(WAILS) build $(WAILS_TAG_ARGS)

test:
	go test ./...

fmt:
	gofmt -w .

check: frontend/node_modules/.package-lock.json frontend/wailsjs/go/models.ts
	cd frontend && npm run check

frontend-test: frontend/node_modules/.package-lock.json
	cd frontend && npm test

verify: test frontend-test check

frontend-install:
	cd frontend && npm ci

frontend/node_modules/.package-lock.json: frontend/package.json frontend/package-lock.json
	cd frontend && npm ci

frontend/wailsjs/go/models.ts: $(WAILS) *.go internal/domain/*.go internal/vault/*.go internal/config/*.go
	$(WAILS) generate module
	./scripts/patch-models.sh

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
