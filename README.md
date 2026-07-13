# NoteVault

[![CI](https://github.com/kvitrvn/notevault/actions/workflows/ci.yml/badge.svg)](https://github.com/kvitrvn/notevault/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

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

- Rich Markdown editing with tables, tasks, code blocks, and local images.
- Full-text search, filters, tags, folders, and pinned notes.
- Wiki links, navigation suggestions, and backlinks.
- Autosave with visible status, plus manual save and draft recovery.
- Version history with diffs and restore, as well as a recoverable trash.
- Templates, themes, daily notes, local statistics, and ZIP export.
- Detection of changes made to vault files outside NoteVault.
- Creation, opening, and instant switching between vaults, with up to eight
  recent vaults.
- Optional passphrase-based encryption for notes, history, and recovery drafts.

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
    └── pins.json
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

## Development

### Prerequisites

- Go 1.25
- Node.js 22 or another current LTS release
- npm
- The [Wails v2 platform dependencies](https://wails.io/docs/gettingstarted/installation)
  for your operating system

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

NoteVault is available under the [MIT License](LICENSE).

Copyright © 2026 Benjamin Gaudé.
