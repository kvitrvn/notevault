# NoteVault

NoteVault est une application desktop de prise de notes locale-first. Les
notes restent des fichiers Markdown lisibles dans un coffre local ; aucun
compte, serveur distant ou service de synchronisation n'est nécessaire.

## Fonctionnalités

- Éditeur Markdown riche avec tableaux, tâches, code et images locales.
- Recherche plein texte SQLite, filtres, tags, dossiers et notes épinglées.
- Liens wiki, suggestions de navigation et backlinks.
- Autosauvegarde, sauvegarde manuelle et récupération des modifications.
- Historique, comparaison de versions, restauration et corbeille.
- Modèles, thèmes, note quotidienne, statistiques et export ZIP.
- Surveillance des modifications apportées aux fichiers hors de l'application.

Les images distantes ne sont pas chargées automatiquement : elles restent dans
le Markdown mais sont bloquées dans l'éditeur afin de préserver la confidentialité
du coffre. Les fichiers locaux peuvent être importés dans `assets/`.

## Organisation du coffre

Au premier démarrage, NoteVault crée par défaut :

```text
~/NoteVault/
├── notes/
├── assets/
├── templates/
├── themes/
└── .notevault/
    ├── config.json
    └── index.db
```

SQLite est un index secondaire reconstruisible. Les fichiers Markdown restent
la source de vérité.

## Stack

- Go 1.25, Wails 2 et SQLite via `modernc.org/sqlite`.
- Svelte 5, TypeScript, Vite, Tailwind CSS et Tiptap.
- Vitest pour les tests unitaires frontend.

## Développement

Pré-requis : Go 1.25, Node.js LTS, npm et les dépendances système demandées par
Wails pour votre plateforme.

Sous Arch/Omarchy, le paquet `webkit2gtk-4.1` est requis. Le Makefile le détecte
avec `pkg-config` et ajoute automatiquement le tag Wails `webkit2_41` à
`make dev` et `make build`. S'il manque :

```bash
omarchy pkg add webkit2gtk-4.1
```

Sous Hyprland, NoteVault sélectionne XWayland pour WebKitGTK afin d'éviter un
bug du backend GTK Wayland qui tronque la surface après un changement de focus.
Le paquet `xorg-xwayland` doit être présent (il l'est par défaut dans Omarchy).
Pour retester Wayland natif après une mise à jour WebKitGTK :

```bash
NOTEAULT_GDK_BACKEND=wayland make dev
```

```bash
git clone git@github.com:kvitrvn/notevault.git
cd notevault
make dev
```

`make dev` installe le CLI Wails attendu et laisse Wails installer les
dépendances frontend si nécessaire.

Commandes utiles :

```bash
make test           # tests Go
make frontend-test  # tests TypeScript avec Vitest
make check          # vérification Svelte et TypeScript
make verify         # les trois vérifications précédentes
make build          # application desktop de production
make regen          # bindings Wails après modification d'une API Go exposée
make fmt            # formatage Go
```

Les cibles frontend utilisent `npm ci` et régénèrent les bindings Wails si le
checkout ne les contient pas. Les fichiers générés sous `frontend/wailsjs/` et
les artefacts sous `frontend/dist/` ne doivent pas être édités à la main.

## Architecture

- `main.go` et `app.go` : démarrage Wails et façade exposée au frontend.
- `internal/domain/` : modèles échangés avec l'interface.
- `internal/config/` : configuration persistée dans le coffre.
- `internal/vault/` : fichiers, index, corbeille, historique, assets et recovery.
- `frontend/src/` : interface Svelte et composants desktop.
- `scripts/` : génération et correctifs des bindings Wails.

La vision produit, ses principes et ses non-objectifs sont détaillés dans
[PRODUCT.md](PRODUCT.md).
