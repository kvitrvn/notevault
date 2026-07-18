# Third-party notices

NoteVault's original source code is licensed under the MIT License. The desktop
binary also links third-party libraries whose licenses continue to apply.

## go-anon

- Project: <https://github.com/Bornholm/go-anon>
- Version: `v0.0.6-0.20260717211550-31cfe3d8c91a`
- License: GNU General Public License v3.0

Because go-anon is linked into the desktop executable, distributions of that
combined executable must comply with GPL-3.0. The complete corresponding
NoteVault source is this repository; go-anon's source is available from the
project and version above. A copy of GPL-3.0 must accompany distributed
binaries.

## Amoxtli

- Project: <https://github.com/Bornholm/amoxtli>
- Version: `v0.1.0`
- License: MIT

NoteVault uses Amoxtli's Markdown parser and Bleve adapter to create an
ephemeral, in-memory retrieval index for the notes selected by the user.

## go-keyring

- Project: <https://github.com/zalando/go-keyring>
- Version: `v0.2.8`
- License: MIT

NoteVault uses go-keyring to create, read, replace, and delete remote chat API
keys in the operating system credential store. Its Linux implementation uses
the Secret Service API over D-Bus.
