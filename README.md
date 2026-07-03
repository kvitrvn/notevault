# NoteVault

Application de prise de notes locale-first : les notes sont des fichiers Markdown dans un coffre personnel. Aucune synchronisation, aucun compte et aucun service distant ne sont nécessaires.

## Pré-requis

- Go 1.23+
- Node.js LTS + npm
- Wails v2 : `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

## Démarrer

```bash
git clone <votre-depot> notevault
cd notevault
go mod tidy
cd frontend && npm install && cd ..
wails dev
```

Au premier démarrage, l'application crée le coffre personnel :

```text
~/NoteVault/
├── notes/
├── assets/
├── templates/
└── .notevault/
```

## Limites volontairement assumées du starter

- Éditeur Markdown brut, sans prévisualisation.
- Pas encore d'index SQLite ni de recherche plein texte.
- Pas encore de sélection graphique du coffre.
- Aucun plugin chargé à ce stade.

Ces limites sont intentionnelles : elles gardent le noyau de fichiers stable avant l'ajout d'indexation, de CodeMirror et de plugins WebAssembly.
