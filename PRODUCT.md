# PRODUCT.md

## Summary

NoteVault is a local-first desktop note-taking app for single-user use. It
stores notes as Markdown files in a local vault and uses an in-memory index for
fast search and navigation. The product goal is a reliable, quiet, fast tool
for writing, finding, organizing, and recovering notes without accounts or
remote services.

## Product Principles

- Local-first: the user's data stays on their machine.
- Readable files by default: notes are real `.md` files. Optional whole-vault
  encryption deliberately makes their contents opaque until the vault is
  unlocked or encryption is disabled.
- Reliability before feature depth: no data loss, atomic operations, trash,
  history, and draft recovery.
- Pragmatic performance: the target vault size is roughly 10,000 notes with
  fluid search and navigation.
- Simplicity: no cloud, collaboration, user accounts, telemetry, marketplace, or
  plugin architecture unless explicitly requested.

## Current Features

- Starts without creating a vault and provides a dedicated vault chooser.
- Creates readable Markdown or encrypted vaults, opens existing vaults, and
  switches between them without restarting.
- Keeps up to eight recent vaults in an application-wide local configuration;
  forgetting a recent entry never deletes its files.
- Local vault with `notes/`, `assets/`, `templates/`, `themes/`, and
  `.notevault/` metadata.
- Create, read, edit, rename, move, duplicate, and delete notes.
- Autosave with visible status and manual save.
- Trash with restore and empty actions.
- In-memory index, search, filters, pinned notes, tags, and folder view.
- Optional daily note.
- User note templates.
- Wiki links, backlinks, and quick navigation.
- Import, storage, and display of local assets.
- Remote images preserved as Markdown but blocked from automatic loading.
- Version history, diff, and restore.
- Built-in themes and user themes.
- ZIP export, local stats, onboarding, and unsaved buffer recovery.
- Optional whole-vault encryption for notes, history, and draft recovery.
- Onboarding shown at most once per process unless reopened manually, with
  recovery taking priority and a global automatic-display preference.

## Non-Goals

- Cloud sync or multi-device sync.
- Accounts, authentication, sharing, or collaboration.
- Hosted web application.
- Plugins or third-party code execution.
- Remote database.

## Data And Security

The vault is the trust boundary. User input includes paths, titles, Markdown
content, assets, templates, and themes. Every feature must preserve vault
confinement, prevent path traversal, and avoid exposing local content to any
remote service by default.

Encryption is local and optional. It does not conceal filenames, directories,
pin metadata, file sizes and dates, or assets. Plaintext necessarily exists in
process and WebView memory while the vault is unlocked, and ZIP exports contain
plaintext Markdown. There is no recovery key in the first version.

Application-wide state is limited to the active vault path, up to eight recent
vault paths with their last-opened times, the format version, and the onboarding
display preference. It lives in the operating system's configuration directory,
outside every vault. Passphrases and encryption keys are never stored there.
