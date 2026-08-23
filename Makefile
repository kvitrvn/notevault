WAILS_VERSION ?= v2.12.0
WAILS := tools/wails/bin/wails
# Arch/Omarchy et les distributions Linux récentes fournissent WebKitGTK 4.1.
# Wails utilise encore 4.0 par défaut, sauf avec ce tag officiel.
WAILS_TAGS ?= $(shell pkg-config --exists webkit2gtk-4.1 2>/dev/null && echo webkit2_41)
WAILS_TAG_ARGS := $(if $(WAILS_TAGS),-tags $(WAILS_TAGS),)
WAILS_BUILD_FLAGS ?= -trimpath
APP_VERSION ?= $(shell ./scripts/build-version.sh)
WAILS_LDFLAGS ?= -X main.buildVersion=$(APP_VERSION)

# Le CLI Wails v2.12.0 embarque golang.org/x/tools v0.30.0, dont le lecteur de
# données d'export ne comprend pas celles produites par Go >= 1.27 : go/packages
# charge alors « time » sans types et la génération échoue sur
# `internal error: package "time" without types was imported from ...`.
# On épingle donc le toolchain sur la version déclarée dans go.mod pour tout ce
# qui passe par le CLI. À retirer quand Wails relèvera x/tools.
GO_TOOLCHAIN ?= $(shell awk '$$1 == "go" { print "go" $$2; exit }' go.mod)
WAILS_GO_ENV := GOTOOLCHAIN=$(GO_TOOLCHAIN)

.PHONY: dev regen build test test-race test-pdf-integration frontend-test fmt fmt-check vet vuln check verify patch-models frontend-install

# Workflow dev : `wails dev` régénère les bindings à chaque démarrage,
# ce qui écrase la classe Time corrigée. Le hook `frontend:dev:watcher`
# dans wails.json (scripts/with-patch-dev.sh) ré-applique le patch juste
# avant de lancer Vite, donc les éditions TypeScript voient la bonne classe.
dev: $(WAILS)
	$(WAILS_GO_ENV) $(WAILS) dev $(WAILS_TAG_ARGS) -ldflags "$(WAILS_LDFLAGS)"

# À utiliser après avoir modifié du code Go exposé : régénère les
# bindings puis applique le patch.
regen: $(WAILS)
	$(WAILS_GO_ENV) $(WAILS) generate module
	./scripts/patch-models.sh

build: $(WAILS)
	$(WAILS_GO_ENV) $(WAILS) build $(WAILS_TAG_ARGS) $(WAILS_BUILD_FLAGS) -ldflags "$(WAILS_LDFLAGS)"

test:
	go test ./...

# Le code a un watcher, un bus de changements, un swap de session et
# plusieurs RWMutex : la version sérielle seule ne prouve pas grand-chose.
test-race:
	go test -race ./...

test-pdf-integration:
	NOTEAULT_PDF_INTEGRATION=1 go test . -run '^TestPDFWorkerWithRealChromium$$'

fmt:
	gofmt -w .

# `fmt` écrit ; `fmt-check` échoue au lieu de corriger, pour la CI.
fmt-check:
	@unformatted=$$(gofmt -l . | grep -v '^frontend/' || true); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt requis sur :"; echo "$$unformatted"; exit 1; \
	fi

vet:
	go vet ./...

# govulncheck est épinglé : en `@latest`, une publication amont casse la CI sans
# qu'aucun commit du dépôt ne change. La base de vulnérabilités, elle, reste
# téléchargée à chaque exécution — l'épinglage ne fige que le moteur d'analyse.
GOVULNCHECK_VERSION ?= v1.7.0

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

check: frontend/node_modules/.package-lock.json frontend/wailsjs/go/models.ts
	cd frontend && npm run check

frontend-test: frontend/node_modules/.package-lock.json frontend/wailsjs/go/models.ts
	cd frontend && npm test

verify: fmt-check vet test-race frontend-test check

frontend-install:
	cd frontend && npm ci

frontend/node_modules/.package-lock.json: frontend/package.json frontend/package-lock.json
	cd frontend && npm ci

frontend/wailsjs/go/models.ts: $(WAILS) *.go internal/chat/*.go internal/domain/*.go internal/vault/*.go internal/config/*.go
	$(WAILS_GO_ENV) $(WAILS) generate module
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
	$(WAILS_GO_ENV) GOBIN=$(PWD)/tools/wails/bin go install github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_VERSION)
