# NoteVault

NoteVault est une application desktop de prise de notes locale-first. Les
notes restent des fichiers Markdown lisibles dans un coffre local ; aucun
compte, serveur distant ou service de synchronisation n'est nécessaire.

Le projet est distribué sous licence [MIT](LICENSE) par Benjamin Gaudé
<kvitrvn@proton.me>. Le code source et les versions publiées sont disponibles
sur <https://github.com/kvitrvn/notevault>.

## Installation Linux

Les paquets officiels sont publiés uniquement dans les GitHub Releases et
ciblent les machines `x86_64`/`amd64`. La première version n'utilise ni AUR ni
dépôt APT et les paquets ne sont pas encore signés par GPG.

Pour Arch Linux, Manjaro et Omarchy :

```bash
sudo pacman -U ./notevault-0.1.0-1-x86_64.pkg.tar.zst
```

Pour Debian 12 ou 13 et Ubuntu 24.04 ou version ultérieure :

```bash
sudo apt install ./notevault_0.1.0_amd64.deb
```

Téléchargez `SHA256SUMS` dans le même dossier que les paquets, puis vérifiez
leur intégrité avant l'installation :

```bash
sha256sum --check SHA256SUMS
```

## Fonctionnalités

- Éditeur Markdown riche avec tableaux, tâches, code et images locales.
- Recherche plein texte en mémoire, filtres, tags, dossiers et notes épinglées.
- Liens wiki, suggestions de navigation et backlinks.
- Autosauvegarde, sauvegarde manuelle et récupération des modifications.
- Historique, comparaison de versions, restauration et corbeille.
- Modèles, thèmes, note quotidienne, statistiques et export ZIP.
- Surveillance des modifications apportées aux fichiers hors de l'application.
- Chiffrement optionnel du contenu des notes, de l’historique et du brouillon
  de récupération avec une phrase secrète locale.
- Création, ouverture et changement immédiat de coffre, avec une liste de huit
  coffres récents au maximum.

Les images distantes ne sont pas chargées automatiquement : elles restent dans
le Markdown mais sont bloquées dans l'éditeur afin de préserver la confidentialité
du coffre. Les fichiers locaux peuvent être importés dans `assets/`.

## Premier lancement et coffres

Au premier démarrage, NoteVault ne crée aucun dossier automatiquement. L’écran
« Choisir un coffre » permet de créer un coffre ou d’ouvrir un coffre NoteVault
existant. Un ancien `~/NoteVault` est repris uniquement s’il contient déjà des
données utiles ; l’arborescence vide créée par une ancienne version est ignorée.

La création propose deux protections :

- **Markdown lisible**, sélectionné par défaut et compatible avec les autres
  éditeurs ;
- **Coffre chiffré**, protégé par une phrase secrète locale sans mécanisme de
  récupération.

Le sélecteur de la barre latérale permet de changer de coffre sans redémarrer.
Les huit derniers coffres sont conservés dans la configuration globale du
système. Retirer un coffre des récents ne supprime jamais son dossier.

Un coffre contient :

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

L’index est reconstruit en mémoire. Les fichiers Markdown restent
la source de vérité. Lorsque le chiffrement est activé, leur extension reste
`.md`, mais leur contenu n’est lisible qu’après déverrouillage dans NoteVault.
Les noms de fichiers, l’arborescence, les épingles et les assets ne sont pas
chiffrés. Un export ZIP produit toujours du Markdown en clair.

La phrase secrète n’est jamais enregistrée et il n’existe pas de clé de
secours : une phrase oubliée rend les notes irrécupérables. L’activation retire
les anciens fichiers `index.db`, sans pouvoir garantir leur effacement
forensique sur un SSD, un snapshot ou une sauvegarde.

Le guide de prise en main est proposé après l’ouverture ou le déverrouillage,
après une éventuelle récupération de brouillon. Il reste disponible depuis
« Raccourcis » et la case « Ne plus afficher automatiquement » contrôle son
affichage lors des prochains lancements.

## Stack

- Go 1.25 et Wails 2.
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

## Publier une version

Les versions utilisent exclusivement des tags SemVer stricts de la forme
`vMAJOR.MINOR.PATCH`. Le tag est la source de vérité de la version des paquets.
Pour publier `v0.1.0`, créez et poussez un tag annoté :

```bash
git tag --annotate v0.1.0 --message "NoteVault v0.1.0"
git push origin v0.1.0
```

Le workflow construit le binaire sous Debian 12, crée les paquets Arch et
Debian, les installe sur toutes les distributions prises en charge, exécute un
smoke test graphique, puis publie la GitHub Release avec `SHA256SUMS`. Un
déclenchement manuel du workflow conserve les mêmes fichiers comme artefacts
sans créer de release.

Avant le premier tag, le paquet doit aussi être testé manuellement sur
Omarchy/Hyprland et Manjaro : installation, présence dans le lanceur, icône et
regroupement de fenêtre, focus, redimensionnement, ouverture d'un coffre, puis
désinstallation sans suppression des données.

Les cibles frontend utilisent `npm ci` et régénèrent les bindings Wails si le
checkout ne les contient pas. Les fichiers générés sous `frontend/wailsjs/` et
les artefacts sous `frontend/dist/` ne doivent pas être édités à la main.

## Architecture

- `main.go` et `app.go` : démarrage Wails et façade exposée au frontend.
- `internal/domain/` : modèles échangés avec l'interface.
- `internal/config/` : configuration persistée dans le coffre.
- `internal/appconfig/` : configuration globale des coffres récents et du guide.
- `internal/vault/` : fichiers, index, corbeille, historique, assets et recovery.
- `frontend/src/` : interface Svelte et composants desktop.
- `scripts/` : génération et correctifs des bindings Wails.

La vision produit, ses principes et ses non-objectifs sont détaillés dans
[PRODUCT.md](PRODUCT.md).
