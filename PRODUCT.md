# PRODUCT.md

## Summary

NoteVault is a local-first desktop note-taking app for single-user use. It
stores notes as Markdown files in a local vault and uses SQLite indexing for
fast search and navigation. The product goal is a reliable, quiet, fast tool
for writing, finding, organizing, and recovering notes without accounts or
remote services.

## Product Principles

- Local-first: the user's data stays on their machine.
- Readable files: notes are real `.md` files, not an opaque proprietary format.
- Reliability before feature depth: no data loss, atomic operations, trash,
  history, and draft recovery.
- Pragmatic performance: the target vault size is roughly 10,000 notes with
  fluid search and navigation.
- Simplicity: no cloud, collaboration, user accounts, telemetry, marketplace, or
  plugin architecture unless explicitly requested.

## Current Features

- Local vault with `notes/`, `assets/`, `templates/`, `themes/`, and
  `.notevault/` metadata.
- Create, read, edit, rename, move, duplicate, and delete notes.
- Autosave with visible status and manual save.
- Trash with restore and empty actions.
- SQLite index, search, filters, pinned notes, tags, and folder view.
- Optional daily note.
- User note templates.
- Wiki links, backlinks, and quick navigation.
- Import, storage, and display of local assets.
- Version history, diff, and restore.
- Built-in themes and user themes.
- ZIP export, local stats, onboarding, and unsaved buffer recovery.

## Non-Goals

- Cloud sync or multi-device sync.
- Accounts, authentication, sharing, or collaboration.
- Built-in encryption.
- Hosted web application.
- Plugins or third-party code execution.
- Remote database.

## Data And Security

The vault is the trust boundary. User input includes paths, titles, Markdown
content, assets, templates, and themes. Every feature must preserve vault
confinement, prevent path traversal, and avoid exposing local content to any
remote service by default.
