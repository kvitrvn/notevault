# AGENTS.md

Short guide for AI coding agents working in this repository. Read
[PRODUCT.md](PRODUCT.md) before proposing or implementing product changes.

## Product

NoteVault is a local-first desktop note-taking app. Notes, assets, templates,
themes, indexes, and recovery state live in a local vault. Do not add accounts,
cloud services, telemetry, remote servers, sync, or plugins unless explicitly
requested.

## Stack

- Go 1.25, Wails v2, and an in-memory vault index.
- Svelte 5, Vite, TypeScript, Tailwind CSS, Tiptap.
- The Wails CLI is installed locally in `tools/wails/bin/wails`.
- Default vault path: `~/NoteVault/`.

## Code Map

- `main.go`, `app.go`: Wails bootstrap and frontend-facing facade. Keep this
  layer thin.
- `internal/domain/`: models exchanged between Go and the frontend.
- `internal/config/`: persisted configuration in `.notevault/config.json`.
- `internal/vault/`: vault business logic, files, index, trash, templates,
  themes, assets, history, stats, and recovery.
- `frontend/src/`: Svelte UI and generated Wails calls.
- `scripts/`: Wails binding patches, especially for `models.ts`.

## Useful Commands

- `make dev`: run Wails in development mode.
- `make test`: apply the Wails patch, then run `go test ./...`.
- `make check`: apply the Wails patch, then run `npm run check`.
- `make build`: build the Wails desktop app.
- `make regen`: regenerate bindings after changing exposed Go APIs.
- `make fmt`: run `gofmt -w .`.

## Working Rules

- Keep tasks short, idiomatic, and easy to review.
- Inspect existing patterns before editing. Do not invent a new architecture.
- No over-engineering: no framework, event bus, generic layer, helper package,
  or speculative abstraction without a concrete need.
- Keep changes close to the requested problem.
- Do not change the vault format or Wails API without a clear product reason.
- If you touch an exposed method in `app.go`, verify Wails bindings and the
  frontend consumer.
- Do not hand-edit generated files; use `make regen` or the existing scripts.

## Go

- Write simple, explicit code with clear error handling.
- Wrap errors with `%w`; do not panic for recoverable failures.
- Keep `App` as a thin facade; business logic belongs in `internal/vault`.
- Use `context.Context` as the first argument for long-running, cancellable, or
  lifecycle-bound operations.
- Close resources and keep goroutine/watcher lifetimes bounded.
- Prefer the standard library. A dependency must clearly reduce risk or
  maintenance cost.
- Add focused tests for parsing, paths, indexing, deletion, restoration, assets,
  migrations, and error paths.

## Frontend

- Build a dense, clear, fast desktop UI; do not create a landing page.
- Reuse existing components and state patterns in `frontend/src/components`.
- Keep TypeScript strict and use generated Wails types.
- Critical interactions need visible states: saving, errors, loading, deletion,
  and restoration.
- Do not keep durable business logic only in the frontend.

## Security

- Apply OWASP principles even for a local app: strict validation, least
  privilege, safe output, and non-verbose errors.
- Treat all vault content as untrusted: Markdown, HTML, images, paths,
  filenames, templates, and themes.
- Prevent path traversal: every user-controlled path must stay relative to the
  vault, normalized, and validated.
- Never serve or open files outside the vault without explicit validation.
- The local asset server must stay confined to `assets/`, bound to loopback,
  and limited to allowed types/extensions.
- Do not log secrets, tokens, personal paths, or note content.
- Use atomic writes for important persisted data.

## Verification

- Go change: `make test`.
- Frontend change: `make check`.
- Wails API change: `make regen`, then `make test` and `make check`.
- Documentation-only change: Markdown review is enough.
