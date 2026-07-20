# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

NoteVault also ships an `AGENTS.md` with the same intent; read it too if this file is truncated or summarized. Read `PRODUCT.md` before proposing or implementing product changes — it defines the local-first vision, principles, and explicit non-goals that should override feature requests that conflict with them.

## What this is

NoteVault is a local-first desktop note-taking app (Go + Wails v2 backend, Svelte 5 frontend). Notes, assets, templates, themes, and recovery state live as plain files in a user vault (default `~/NoteVault/`); the in-memory search index is disposable and always rebuilt from the vault. Do not add accounts, cloud services, telemetry, remote servers, sync, or plugins unless explicitly requested — these are product non-goals, not oversights.

## Commands

- `make dev` — run the app in development mode (installs the pinned Wails CLI into `tools/wails/bin/` on first run).
- `make test` — regenerate `frontend/wailsjs/go/models.ts` if stale, then `go test ./...`.
- `make frontend-test` — run frontend unit tests with Vitest (`cd frontend && npm test`).
- `make check` — Svelte/TypeScript type checking (`cd frontend && npm run check`).
- `make verify` — `test` + `frontend-test` + `check`; run this before considering a change done.
- `make build` — build the production desktop binary.
- `make regen` — after changing any exposed Go API (methods on `App`, or types in `internal/domain`, `internal/chat`, `internal/vault`, `internal/config`): regenerates Wails bindings then re-applies the `models.ts` patch.
- `make fmt` — `gofmt -w .`.

Single test: `go test ./internal/vault/ -run TestName -v`. Frontend single test: `cd frontend && npx vitest run src/lib/assets.test.ts`.

Never hand-edit `frontend/wailsjs/` (generated bindings) or `frontend/dist/` (build output).

### Wails codegen quirk

Wails' generator emits an empty `Time` class in `frontend/wailsjs/go/models.ts` that doesn't round-trip dates (Go receives `{}` and fails to deserialize). `scripts/patch-models.sh` patches in a `toJSON()`. This runs automatically via the Makefile and via `wails.json` dev-watcher hooks (`scripts/with-patch-dev.sh`) — if you touch bindings manually or bypass `make`, re-run `make patch-models`.

## Architecture

### Backend layering

```
main.go, app.go            Wails bootstrap + frontend-facing facade (App). Keep thin.
internal/domain/           Models shared between Go and the frontend (serialized via Wails bindings)
internal/appconfig/        App-wide config: active/recent vault paths, onboarding flag — lives in the OS config dir, outside any vault
internal/config/           Per-vault configuration (.notevault/config.json)
internal/vault/            Vault business logic: files, in-memory index, history, trash, templates, themes, assets, stats, encryption, recovery, filesystem watcher
internal/chat/             Opt-in note chat: ephemeral retrieval index, local anonymization, provider calls, secret storage
```

`App` (app.go) holds an optional `vaultSession` (nil until a vault is opened) wrapping the `vault.Service`, its `AssetServer`, and a lazily-created `chat.Service`. Vault switching goes through `switchMu`/`switching` to serialize concurrent open/close. Business logic belongs in `internal/vault` and `internal/chat`, not in `App` — if you're adding a method to `app.go` beyond argument marshaling and session lookup, it likely belongs one layer down.

Any method added to `App` that should be callable from the frontend must go through `make regen`, and its frontend caller in `frontend/src/` must be updated to match the regenerated bindings.

### Chat feature (internal/chat)

Chat is opt-in and never automatic. Flow: user selects notes or tags → tags resolve to explicit paths locally (never sent as a filter) → an ephemeral in-memory Amoxtli retrieval index is built (nothing persisted) → retrieved passages and the question are anonymized locally with go-anon, replacing note paths/titles with stable per-conversation `SOURCE_n` identifiers → the user reviews the exact anonymized payload before any network call. Ollama calls are restricted to `127.0.0.1:11434`; remote mode supports fixed OpenAI/Mistral/OpenRouter endpoints only, never enabled automatically. Remote API keys stay in process memory by default and are persisted only with explicit user consent, one entry per provider in the OS credential store (Secret Service/keyring via `internal/chat/secrets.go`) — never in `app.json`, the vault, or logs. Chat is unavailable for encrypted vaults (no plaintext-derived index may exist alongside vault encryption).

### Frontend

`frontend/src/App.svelte` is the shell; feature UI lives in `frontend/src/components/`, cross-cutting logic (asset handling, chat selection/settings, vault manager, wiki-link parsing) in `frontend/src/lib/`. Generated Wails call stubs live in `frontend/src/wailsjs/` (via `frontend/wailsjs/`) — treat as read-only output of `make regen`.

### Vault format

```
~/NoteVault/
├── notes/
├── assets/
├── templates/
├── themes/
└── .notevault/
    ├── config.json
    ├── pins.json
    └── chat/models/     # downloaded go-anon language models, no note index
```

Markdown files under `notes/` are always the source of truth; the search index is derived and rebuildable. Two storage modes exist: readable Markdown (default) and passphrase-encrypted (no recovery key — losing the passphrase makes notes unrecoverable). Encryption never conceals filenames, directory structure, pin metadata, file sizes/dates, or assets. Do not change the vault format or the Wails-exposed API without a clear product reason — both are effectively durable, user-facing contracts.

## Security boundaries (load-bearing, not optional)

- The vault directory is the trust boundary. Every user-controlled path must be normalized and validated to stay inside it; never open or serve files outside the vault without explicit validation. Treat all vault content (Markdown, HTML, images, paths, filenames, templates, themes) as untrusted input.
- The local asset server (`internal/vault/asset_server.go`) must stay confined to `assets/`, bind only to loopback, and allow only supported file types/extensions.
- Never log secrets, tokens, personal paths, or note/chat content.
- Use atomic writes for persisted state that matters (config, index metadata, vault files).
- Outbound chat calls are the one intentional network boundary — see the Chat section above; don't widen it (new endpoints, automatic remote calls, persisted plaintext derivatives) without an explicit product decision.

## Working conventions

- Keep changes small, idiomatic, and close to the requested problem; inspect existing patterns before introducing a new one. No speculative abstractions, frameworks, or generic layers without a concrete need.
- Go: wrap errors with `%w`, don't panic for recoverable failures, use `context.Context` as the first argument for cancellable/lifecycle-bound operations, keep goroutine/watcher lifetimes bounded and resources closed. Prefer the standard library — a new dependency needs a clear risk/maintenance justification.
- Add focused tests for parsing, paths, indexing, deletion/restoration, assets, migrations, and error paths.
- Frontend: reuse existing components/state patterns, keep TypeScript strict, use generated Wails types. Critical interactions (saving, errors, loading, deletion, restoration) need visible state — this is a dense desktop UI, not a landing page. Don't let durable business logic live only in the frontend.

## Verification before calling a change done

- Go-only change: `make test`.
- Frontend-only change: `make check` (and `make frontend-test` if behavior changed).
- Any change to an `App`-exposed method or shared domain type: `make regen`, then `make test` and `make check`.
- Docs-only change: a Markdown read-through is enough.
