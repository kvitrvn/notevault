# NoteVault Product Overview

## Vision

NoteVault is a quiet, dependable desktop application for writing, finding,
organizing, and recovering personal notes. It offers the convenience of a
modern note-taking interface while preserving the durability and portability
of files stored on the user's own computer.

The product is local-first and designed for one person. It does not require an
account, network connection, remote database, or hosted service.

## Product promise

NoteVault keeps the vault under the user's control:

- Notes are stored locally as Markdown files by default.
- The vault remains the source of truth, not an internal database.
- Search and navigation stay fast through an index rebuilt in memory.
- Important editing operations favor recovery and data safety.
- No vault content is sent to a remote service by default.

Optional vault encryption deliberately changes the first promise: note files
keep their `.md` extension, but their contents remain opaque until the vault is
unlocked or encryption is disabled.

## Intended users

NoteVault is for individuals who want a focused desktop workspace and direct
ownership of their notes. It is especially suited to people who:

- prefer local files over a hosted notes service;
- want Markdown portability without giving up a rich editor;
- manage personal notes, references, journals, or small knowledge bases;
- value predictable behavior, privacy, and recovery over collaboration or a
  large extension ecosystem.

## Product principles

### Local-first

User data stays on the user's machine. Features must work without an account or
remote service and should remain useful without a network connection.

### Files users can own

Readable Markdown is the default. Notes should remain accessible with ordinary
tools outside NoteVault. Encryption is optional and its limitations must be
communicated clearly.

### Reliability before feature depth

Avoiding data loss matters more than adding breadth. Atomic writes, autosave,
trash, version history, and draft recovery are core product behavior rather
than secondary features.

### Fast enough for a real vault

The working target is approximately 10,000 notes with fluid search and
navigation. The in-memory index is disposable and must be reconstructible from
the vault.

### Deliberate simplicity

NoteVault should remain focused and understandable. New infrastructure,
abstractions, or product surfaces must solve a demonstrated user problem.

### Privacy by default

Vault content is untrusted and private. NoteVault must avoid unexpected network
requests, keep file access inside the vault boundary, and expose no content to
third parties by default.

## Current product experience

### Vault lifecycle

- Start without silently creating a vault.
- Create a readable Markdown vault or an encrypted vault.
- Open an existing vault and switch vaults without restarting.
- Remember up to eight recent vaults in local application-wide configuration.
- Forgetting a recent vault entry never deletes its files.

### Writing and organization

- Create, read, edit, rename, move, duplicate, and delete notes.
- Use a rich Markdown editor with autosave status, manual save, and
  Markdown-aware plain-text paste.
- Organize with folders, tags, pinned notes, filters, and an optional daily
  note.
- Create notes from user-defined templates.
- Navigate with wiki links, backlinks, and quick suggestions.

### Search and navigation

- Build the search index in memory from vault contents.
- Provide full-text search and responsive navigation across the target vault
  size.
- Detect relevant file changes made outside the application.

### Private note chat

- Start a conversation from explicit notes or tags, while always showing and
  allowing adjustment of the resolved note list before retrieval.
- Retrieve relevant Markdown sections locally through an ephemeral in-memory
  index and keep note files as the sole source of truth.
- Anonymize the question and retrieved sections locally, with stable
  pseudonyms for the lifetime of the conversation.
- Require review of the exact anonymized payload before every model call.
- Support loopback-only Ollama and explicit remote OpenAI, Mistral, or
  OpenRouter calls; remote mode is never enabled automatically.
- Keep pseudonym mappings in process memory only. API keys remain ephemeral by
  default and may be saved only with explicit consent in the operating
  system's credential store, never in the vault or ordinary configuration.
- Keep the first chat version unavailable for encrypted vaults rather than
  persisting plaintext-derived state.

### Assets and external content

- Import, store, and display local assets from the vault's `assets/` directory.
- Preserve remote image references in Markdown without loading them
  automatically.
- Treat Markdown, HTML, images, paths, filenames, templates, and themes as
  untrusted input.

### Recovery and portability

- Move deleted notes to trash with restore and empty actions.
- Keep version history, show diffs, and restore earlier versions.
- Recover unsaved editing buffers after interruption.
- Export selected notes as a ZIP containing plaintext Markdown.
- Export the active note as a local PDF through an isolated worker and an
  already installed Chrome or Chromium browser.

### Personalization and insight

- Offer built-in themes and local user themes.
- Provide local vault statistics.
- Display the embedded application version and notify the user when a newer
  stable release is available.
- Show onboarding at most once per process unless reopened manually.
- Give draft recovery priority over automatic onboarding.
- Store the onboarding display preference outside the vault.

## Vault model

A standard vault contains:

```text
vault/
├── notes/
├── assets/
├── templates/
├── themes/
└── .notevault/
    └── pdf-themes/       # optional declarative PDF themes
```

Notes and their related data live in this local directory. The in-memory index
is derived state and can be rebuilt. The application may keep only limited
global state outside the vault:

- the active vault path;
- up to eight recent vault paths and their last-opened times;
- the application configuration format version;
- the automatic onboarding display preference;
- the last chat provider and the last model selected for each supported
  provider.

This global state lives in the operating system's configuration directory.
Passphrases, encryption keys, and chat API keys are never stored there. Chat
API keys explicitly remembered by the user live in the system credential store
under one entry per remote provider.

## Security and privacy boundaries

The vault is the primary trust boundary. Every user-controlled path must remain
relative to it after normalization and validation. Features must prevent path
traversal and must not open or serve files outside the vault without explicit,
validated intent.

The local asset server must remain confined to `assets/`, bind only to loopback,
and allow only supported file types. Logs must not contain secrets, note
content, or unnecessary personal paths. Important persisted state should use
atomic writes.

PDF export creates its HTML inside NoteVault, escapes raw HTML, embeds only
validated raster assets from `assets/`, and applies a restrictive content
security policy. It must not resolve remote or `file://` resources. Amatl is
limited to HTML-to-PDF conversion inside a child worker; its templates,
directives, Markdown processing, and URL resolvers are not exposed to notes or
themes. A parent timeout terminates the worker and its Chromium process group.

Packaged builds make one automatic, unauthenticated HTTPS request per process
start to GitHub's fixed `kvitrvn/notevault/releases/latest` API endpoint. The
endpoint returns the latest stable release and excludes drafts and
prereleases. The request has a five-second timeout and sends no vault path,
note content, account, or user identifier. A failure never blocks startup and
is shown only as a discreet retry state beside the installed version. Clicking
the version explicitly retries the check and may therefore make another
request. Local `dev` builds skip the request. GitHub associates unauthenticated
requests with the originating IP address and currently limits them to 60 per
hour. NoteVault never opens a URL supplied by the response: the update action
uses the fixed
`https://github.com/kvitrvn/notevault/releases/latest` page, and package
installation remains manual.

Chat adds an explicit outbound trust boundary. Retrieval and anonymization run
locally. Provider payloads contain only the reviewed anonymized question,
reviewed anonymized passages, and opaque `SOURCE_n` identifiers. Filenames,
paths, API keys, pseudonym mappings, and cleartext conversation history must not
be logged or persisted in ordinary files. The only exception is an API key
explicitly saved in the system credential store. Anonymization is risk
reduction, not a guarantee of complete de-identification.

Remembering a remote provider key always requires explicit consent. On Linux,
NoteVault uses the desktop Secret Service through D-Bus. If that service is
locked or unavailable, remote providers are disabled while loopback-only
Ollama remains usable. The credential store protects secrets at rest, but a
malicious process running in the same unlocked user session may still access
them.

### Encryption scope and limitations

Encryption is local and optional. It covers note contents, history, and draft
recovery data. It does not conceal:

- filenames or directory structure;
- pin metadata;
- file sizes or dates;
- assets.

Plaintext necessarily exists in process and WebView memory while an encrypted
vault is unlocked. ZIP and PDF exports contain plaintext and therefore require
an explicit confirmation. The first version has no recovery key, so a
forgotten passphrase makes encrypted notes unrecoverable.

## Non-goals

The following are outside the current product scope:

- cloud or multi-device synchronization;
- accounts, authentication, sharing, or collaboration;
- a hosted web application;
- telemetry or remote analytics;
- plugins, a marketplace, or third-party code execution;
- a remote database.

These constraints are intentional. They should change only in response to an
explicit product decision, not as an incidental part of another feature.

## Decision filter for new features

Before adding a feature, ask:

1. Does it improve writing, finding, organizing, or recovering notes?
2. Does it preserve local ownership and offline usefulness?
3. Can it stay within the vault's security boundary?
4. Does it introduce a new durable format or exposed API, and is that cost
   justified?
5. Can the problem be solved with a small, reviewable change using existing
   architecture?
6. How does the feature behave during failure, interruption, or recovery?

When a proposal conflicts with reliability, privacy, or clear file ownership,
those principles take precedence over feature breadth.
