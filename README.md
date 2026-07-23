# NoteVault

[![CI](https://github.com/kvitrvn/notevault/actions/workflows/ci.yml/badge.svg)](https://github.com/kvitrvn/notevault/actions/workflows/ci.yml)
[![Source license: MIT](https://img.shields.io/badge/source-MIT-6a737d.svg)](LICENSE)
[![Binary license: GPL-3.0](https://img.shields.io/badge/binary-GPL--3.0-6a737d.svg)](THIRD_PARTY_NOTICES.md)

> [!IMPORTANT]
> NoteVault is under active development. Features, vault behavior, and release
> packaging may change, and the application is not yet recommended as the only
> copy of important notes. Keep regular backups of your vault.

NoteVault is a local-first desktop note-taking application. Your notes remain
ordinary Markdown files in a vault on your computer—no account, remote server,
or synchronization service required.

## Why NoteVault?

NoteVault is built for people who want the speed and convenience of a modern
notes application without giving up ownership of their files. It combines a
rich Markdown editor with fast local search, organization tools, and recovery
features while keeping the vault as the source of truth.

- **Local by default:** notes and application data stay on your machine.
- **Portable files:** unencrypted notes are readable `.md` files that work with
  other editors and tools.
- **Fast navigation:** an in-memory index powers search, filters, tags, folders,
  wiki links, and backlinks.
- **Recovery-minded:** autosave, history, trash, and draft recovery help protect
  work in progress.
- **Private by design:** remote images are not loaded automatically, and no
  account or telemetry is required.

## Features

- Rich Markdown editing with tables, tasks, code blocks, local images, and
  Markdown-aware plain-text paste.
- Full-text search, filters, tags, folders, and pinned notes.
- Wiki links, navigation suggestions, and backlinks.
- Autosave with visible status, plus manual save and draft recovery.
- Version history with diffs and restore, as well as a recoverable trash.
- Templates, themes, daily notes, local statistics, ZIP export, and local PDF
  export of the active note.
- Detection of changes made to vault files outside NoteVault.
- Display of the installed version and notification when a newer stable release
  is available.
- Creation, opening, and instant switching between vaults, with up to eight
  recent vaults.
- Optional passphrase-based encryption for notes, history, and recovery drafts.
- Opt-in chat over an explicit note selection, with local Amoxtli retrieval,
  local go-anon anonymization, a mandatory payload preview, and local or remote
  OpenAI-compatible models.

See [PRODUCT.md](PRODUCT.md) for the product vision, principles, current scope,
and explicit non-goals.

## Installation

Linux packages are published through
[GitHub Releases](https://github.com/kvitrvn/notevault/releases) for
`x86_64`/`amd64` systems. Packages are not currently distributed through AUR
or an APT repository and are not GPG-signed.

### Arch Linux, Manjaro, and Omarchy

```bash
sudo pacman -U ./notevault-0.1.0-1-x86_64.pkg.tar.zst
```

### Debian and Ubuntu

The packages target Debian 12 or 13 and Ubuntu 24.04 or later.

```bash
sudo apt install ./notevault_0.1.0_amd64.deb
```

`gnome-keyring` is recommended to remember remote chat API keys through the
Linux Secret Service. Another Secret Service-compatible credential store can
be used instead. Without an available, unlocked Secret Service, remote chat
providers are disabled and local Ollama remains available.

Chrome or Chromium is optional and is used only to export the active note as a
PDF. NoteVault detects an existing installation but never downloads, bundles,
or installs a browser.

Download `SHA256SUMS` alongside the package and verify its integrity before
installation:

```bash
sha256sum --check SHA256SUMS
```

## Vaults and data ownership

NoteVault does not create a vault on first launch. The vault chooser lets you
create one or open an existing NoteVault vault. A legacy `~/NoteVault` folder
is reused only when it already contains meaningful data; an empty folder from
an earlier version is ignored.

New vaults can use one of two storage modes:

- **Readable Markdown** is the default and remains compatible with other
  editors.
- **Encrypted vault** protects note content with a local passphrase and has no
  recovery mechanism.

A vault has the following structure:

```text
~/NoteVault/
├── notes/
├── assets/
├── templates/
├── themes/
└── .notevault/
    ├── config.json
    ├── pins.json
    ├── pdf-themes/      # optional declarative PDF themes
    └── chat/models/     # downloaded go-anon models; no note index
```

The search index is rebuilt in memory; Markdown files remain the source of
truth. When encryption is enabled, notes keep their `.md` extension but their
contents are readable only after the vault is unlocked in NoteVault.

Encryption does not conceal filenames, directory structure, pin metadata,
file sizes, dates, or assets. ZIP exports always contain plaintext Markdown.
The passphrase is never stored, and there is no recovery key: losing it makes
the encrypted notes unrecoverable. Enabling encryption removes legacy
`index.db` files but cannot guarantee forensic erasure from SSDs, snapshots, or
backups.

Remote images remain in Markdown but are blocked in the editor to avoid
unexpected network requests. Local files can be imported into `assets/`.

## PDF export and themes

The export dialog can create one PDF from the active note. Pending editor
changes are saved first. Rendering stays local in an isolated NoteVault worker
and uses an already installed Chrome or Chromium browser with its sandbox
enabled. The generated HTML has a restrictive content security policy, embeds
only validated raster images from `assets/` as data URLs, and does not load
remote images, styles, fonts, scripts, or other network resources. Raw HTML in
Markdown is displayed as escaped text; Mermaid remains a code block in this
first version.

Custom PDF themes are separate from interface themes. Create JSON files smaller
than 64 KiB in `.notevault/pdf-themes/`; the filename without `.json` is the
theme identifier. A complete version 1 theme looks like this:

```json
{
  "version": 1,
  "page": {
    "size": "A4",
    "orientation": "portrait",
    "margins": {"top": 20, "right": 18, "bottom": 20, "left": 18}
  },
  "typography": {
    "family": "serif",
    "monoFamily": "monospace",
    "bodySizePt": 11,
    "lineHeight": 1.5,
    "headingScale": 1.25
  },
  "colors": {
    "text": "#202124",
    "secondary": "#5f6368",
    "accent": "#315c8c",
    "codeBackground": "#f3f4f6"
  },
  "options": {
    "titlePage": false,
    "metadata": true,
    "pageNumbers": true
  }
}
```

Allowed page sizes are `A4` and `Letter`; margins are 5–40 mm. The body size is
9–18 pt, line height 1.2–2, heading scale 1–2, and colors must be six-digit
hexadecimal values. `family` accepts `serif` or `sans-serif`, while
`monoFamily` accepts `monospace`. Unknown or invalid fields reject the whole
theme. Raw CSS, HTML, JavaScript, templates, URLs, external fonts, and file
paths are not supported.

## Update checks and network privacy

Once per application start, a packaged NoteVault build sends an unauthenticated
HTTPS `GET` request to GitHub's fixed
`kvitrvn/notevault/releases/latest` API endpoint. GitHub defines this endpoint
as the most recent published release that is neither a draft nor a prerelease.
NoteVault sends no vault path, note content, account, or user identifier. Local
`dev` builds do not make this request.

The request times out after five seconds and any network or response error is
ignored, so startup and offline use are unaffected. GitHub associates
unauthenticated API traffic with the originating IP address and currently
limits it to 60 requests per hour. When an update exists, NoteVault only links
to the fixed [GitHub Releases page](https://github.com/kvitrvn/notevault/releases/latest);
installing the `.deb` or `.pkg.tar.zst` package remains manual.

See GitHub's documentation for the
[latest-release endpoint](https://docs.github.com/en/rest/releases/releases#get-the-latest-release)
and [unauthenticated rate limit](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api#primary-rate-limit-for-unauthenticated-users).

## Chat and network privacy

Chat is disabled by default and starts only from the **Chat** action. The user
selects notes directly or adds all notes carrying one or more tags, adjusts the
resolved list, enters a model, and reviews the exact anonymized payload before
sending it. Tag selection is resolved to explicit note paths locally; no tag or
mutable filter is sent to the provider. NoteVault uses an ephemeral in-memory
Amoxtli index; it does not persist note chunks or embeddings. Note paths and
titles are replaced with local `SOURCE_n` identifiers in the provider payload.

The first preview downloads the required French, English, or Spanish go-anon
model from go-anon's HTTPS manifest, verifies its SHA-256 checksum, and caches
it inside `.notevault/chat/models/`. A language model is currently around 200
MiB, so the first preview can take a moment. Anonymization and retrieval always
run locally. Ollama calls are restricted to `127.0.0.1:11434`; remote mode
supports fixed OpenAI, Mistral, and OpenRouter endpoints. API keys remain in
WebView and Go process memory by default. A key can be remembered only after
the user checks the explicit consent option; it is then stored as a
provider-specific entry in the operating system credential store, never in
`app.json`, the vault, logs, or chat history. On Linux this uses Secret Service
through D-Bus. If the credential store is locked or unavailable, remote
providers are disabled; Ollama never accesses it.

Anonymization reduces disclosure risk but cannot guarantee that every sensitive
value is detected. The preview is therefore required for local and remote
providers. Chat is not available for encrypted vaults in this first version,
so no plaintext derived index can weaken vault encryption.

## Development

### Prerequisites

- Go 1.25.5 or later
- Node.js 22 or another current LTS release
- npm
- The [Wails v2 platform dependencies](https://wails.io/docs/gettingstarted/installation)
  for your operating system
- A Secret Service implementation such as GNOME Keyring (optional, recommended
  for remote chat providers on Linux)
- Chrome or Chromium (optional, required only for PDF export)

On Arch Linux and Omarchy, install WebKitGTK 4.1 if needed:

```bash
omarchy pkg add webkit2gtk-4.1
```

The Makefile detects `webkit2gtk-4.1` through `pkg-config` and automatically
adds Wails' `webkit2_41` build tag. Under Hyprland, NoteVault defaults to
XWayland to avoid a WebKitGTK rendering issue after focus changes. To test the
native Wayland backend explicitly:

```bash
NOTEAULT_GDK_BACKEND=wayland make dev
```

### Run locally

```bash
git clone https://github.com/kvitrvn/notevault.git
cd notevault
make dev
```

`make dev` installs the expected Wails CLI into `tools/wails/bin/` when needed.
Frontend dependencies are installed automatically by the development workflow.

### Useful commands

| Command | Purpose |
| --- | --- |
| `make dev` | Run the application in development mode |
| `make test` | Run Go tests |
| `make test-pdf-integration` | Run the PDF integration test with a real Chromium |
| `make frontend-test` | Run frontend unit tests with Vitest |
| `make check` | Check Svelte and TypeScript code |
| `make verify` | Run all tests and frontend checks |
| `make build` | Build the production desktop application |
| `make regen` | Regenerate Wails bindings after an exposed Go API change |
| `make fmt` | Format Go code |

Generated files under `frontend/wailsjs/` and build artifacts under
`frontend/dist/` must not be edited by hand.

## Project structure

```text
.
├── app.go, main.go       Wails bootstrap and frontend-facing facade
├── internal/domain/      Models shared by Go and the frontend
├── internal/config/      Configuration stored inside a vault
├── internal/appconfig/   Application-wide vault and onboarding settings
├── internal/chat/        Ephemeral retrieval, anonymization, and providers
├── internal/vault/       Vault files, index, history, trash, and recovery
├── frontend/src/         Svelte desktop interface and components
└── scripts/              Packaging and Wails binding utilities
```

## Contributing

Contributions, bug reports, and focused feature proposals are welcome. Before
starting a substantial change, read [PRODUCT.md](PRODUCT.md) and open an issue
to discuss how it fits NoteVault's local-first scope.

When submitting a pull request:

1. Keep the change small and focused.
2. Add or update tests for changed behavior.
3. Run `make verify`.
4. Run `make build` when the change affects integration or packaging.

Please report bugs and request features through
[GitHub Issues](https://github.com/kvitrvn/notevault/issues).

## Releasing

Releases use strict SemVer tags in the form `vMAJOR.MINOR.PATCH`. The tag is the
source of truth for package versions. For example:

```bash
git tag --annotate v0.1.0 --message "NoteVault v0.1.0"
git push origin v0.1.0
```

The release workflow builds the binary on Debian 12, creates Arch and Debian
packages, installs them on supported distributions, runs graphical smoke tests,
and publishes a GitHub Release with SHA-256 checksums. A manually triggered
workflow retains the same files as artifacts without creating a release.

## License

NoteVault's original source code is available under the [MIT License](LICENSE).
The desktop executable links GPL-3.0-licensed go-anon and must therefore be
distributed under GPL-3.0-compliant terms. See
[third-party notices](THIRD_PARTY_NOTICES.md) for pinned versions and details.

Copyright © 2026 Benjamin Gaudé.
