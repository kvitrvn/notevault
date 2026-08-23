#!/usr/bin/env bash
# Génère des notes de référence pour mesurer les performances d'affichage et
# de scroll de l'éditeur sur de longs documents.
#
# Une note « full » mélange tous les facteurs de coût (texte, blocs de code,
# diagrammes Mermaid, images, tableaux, wiki-links). Les variantes mono-facteur
# servent à attribuer un coût mesuré à une cause précise plutôt qu'à « la note
# est longue » : on compare `bench-text` et `bench-code` à nombre de lignes
# égal pour isoler le surcoût des blocs de code, etc.
#
# Les notes sont écrites dans un coffre de test dédié — jamais dans le coffre
# par défaut de l'utilisateur, qu'on ne veut pas polluer. Ouvrir ce coffre via
# le sélecteur de coffre de l'application.
#
# Usage :
#   scripts/gen-bench-note.sh                        # toutes les variantes
#   scripts/gen-bench-note.sh --variant text         # une seule
#   scripts/gen-bench-note.sh --vault /tmp/bench --lines 20000
set -euo pipefail

cd "$(dirname "$0")/.."

VAULT="${HOME}/NoteVault-bench"
LINES=5000
VARIANTS=(full text code mermaid images tables)

usage() {
  cat <<'USAGE'
Usage: scripts/gen-bench-note.sh [options]

  --vault DIR      Coffre de destination (défaut: ~/NoteVault-bench)
  --lines N        Nombre de lignes visé par note (défaut: 5000)
  --variant NAME   Une seule variante: full|text|code|mermaid|images|tables
  -h, --help       Cette aide
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --vault) VAULT="$2"; shift 2 ;;
    --lines) LINES="$2"; shift 2 ;;
    --variant) VARIANTS=("$2"); shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Option inconnue: $1" >&2; usage >&2; exit 1 ;;
  esac
done

if ! [[ "$LINES" =~ ^[0-9]+$ ]] || (( LINES < 100 )); then
  echo "--lines doit être un entier >= 100" >&2
  exit 1
fi

NOTES_DIR="$VAULT/notes"
ASSETS_DIR="$VAULT/assets"
mkdir -p "$NOTES_DIR" "$ASSETS_DIR"

# --- Images -----------------------------------------------------------------
# Générées avec image/png de la bibliothèque standard Go : le dépôt dépend déjà
# de Go, alors qu'ImageMagick n'est pas garanti sur la machine de dev.
# Des dimensions variées sont indispensables : le but est justement d'observer
# les reflows dus à l'absence de taille intrinsèque au moment du décodage.
IMAGE_COUNT=8
gen_images() {
  local existing
  existing=$(find "$ASSETS_DIR" -maxdepth 1 -name 'bench-*.png' | wc -l)
  if (( existing >= IMAGE_COUNT )); then
    echo "  images  : $existing déjà présentes dans assets/"
    return
  fi
  echo "  images  : génération de $IMAGE_COUNT PNG dans assets/"
  # `go run` ne lit pas depuis stdin : la source passe par un fichier
  # temporaire, hors du module pour ne pas polluer le graphe de paquets.
  local tmp
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' RETURN
  cat > "$tmp/gen.go" <<'GO_EOF'
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strconv"
)

func main() {
	dir := os.Args[1]
	count, err := strconv.Atoi(os.Args[2])
	if err != nil {
		panic(err)
	}
	// Tailles volontairement hétérogènes, y compris une image large qui
	// déborde de la largeur de lecture.
	sizes := [][2]int{{320, 200}, {640, 400}, {800, 600}, {1200, 500}, {480, 480}}
	for i := 0; i < count; i++ {
		w, h := sizes[i%len(sizes)][0], sizes[i%len(sizes)][1]
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.Set(x, y, color.RGBA{
					R: uint8((x * 255) / w),
					G: uint8((y * 255) / h),
					B: uint8((i * 37) % 255),
					A: 255,
				})
			}
		}
		path := fmt.Sprintf("%s/bench-%02d.png", dir, i)
		f, err := os.Create(path)
		if err != nil {
			panic(err)
		}
		if err := png.Encode(f, img); err != nil {
			panic(err)
		}
		if err := f.Close(); err != nil {
			panic(err)
		}
	}
}
GO_EOF
  go run "$tmp/gen.go" "$ASSETS_DIR" "$IMAGE_COUNT"
}

# --- Fragments --------------------------------------------------------------
# Chaque fonction ajoute ses lignes au tableau global `out`.

WORDS=(coffre note index markdown éditeur diagramme recherche vault local passage
       ancrage rendu défilement latence mesure bloc curseur sélection sauvegarde
       chiffrement historique corbeille modèle template thème asset lien)

para() {
  local n=$1 i line w
  for (( i = 0; i < 3; i++ )); do
    line=""
    for (( w = 0; w < 14; w++ )); do
      line+="${WORDS[$(( (n * 7 + i * 13 + w * 3) % ${#WORDS[@]} ))]} "
    done
    out+=("${line% }")
  done
  out+=("")
}

# Une moitié des cibles pointe vers des notes inexistantes : les deux branches
# de résolution du plugin wiki-link sont ainsi exercées (accent / rouge).
wikipara() {
  local n=$1 target
  if (( n % 2 == 0 )); then
    target="Bench Cible ${n}"
  else
    target="Note Inexistante ${n}"
  fi
  out+=("Voir [[${target}]] et [[Bench Cible $(( n + 1 ))]] pour le détail du sujet ${n}.")
  out+=("")
}

heading() {
  out+=("## Section $1" "")
}

listblock() {
  local n=$1 i
  for (( i = 1; i <= 5; i++ )); do
    out+=("- Élément ${i} de la liste ${n}")
  done
  out+=("")
}

quote() {
  out+=("> Citation ${1} : le document reste la source de vérité, l'index est reconstructible." "")
}

# Le corps doit être syntaxiquement valide dans la langue annoncée : sur une
# entrée invalide, la récupération d'erreur de Lezer coûte plus cher que le
# parse nominal, ce qui fausserait la mesure du surcoût des blocs de code.
CODE_LANGS=(go typescript rust python bash sql)
codeblock() {
  local n=$1 lang i
  lang="${CODE_LANGS[$(( n % ${#CODE_LANGS[@]} ))]}"
  out+=('```'"${lang}")
  for (( i = 0; i < 16; i++ )); do
    case "$lang" in
      go)         out+=("func step${n}_${i}(value int) int { return value*${i} + ${n} }") ;;
      typescript) out+=("export function step${n}_${i}(value: number): number { return value * ${i} + ${n}; }") ;;
      rust)       out+=("pub fn step${n}_${i}(value: i64) -> i64 { value * ${i} + ${n} }") ;;
      python)     out+=("def step${n}_${i}(value): return value * ${i} + ${n}") ;;
      bash)       out+=("step${n}_${i}() { echo \$(( \$1 * ${i} + ${n} )); }") ;;
      sql)        out+=("SELECT path, ${i} AS step FROM notes WHERE id = ${n} AND rank > ${i};") ;;
    esac
  done
  out+=('```' "")
}

mermaidblock() {
  local n=$1
  out+=('```mermaid')
  out+=("flowchart TD")
  out+=("  A${n}[Ouverture note ${n}] --> B${n}[Parse Markdown]")
  out+=("  B${n} --> C${n}[Montage ProseMirror]")
  out+=("  C${n} --> D${n}[Rendu blocs]")
  out+=("  D${n} --> E${n}{Bloc lourd ?}")
  out+=("  E${n} -->|oui| F${n}[CodeMirror / Mermaid]")
  out+=("  E${n} -->|non| G${n}[Flux de texte]")
  out+=('```' "")
}

tableblock() {
  local n=$1 i
  out+=("| Chemin | Rôle | Coût | Note |")
  out+=("| --- | --- | --- | --- |")
  for (( i = 0; i < 8; i++ )); do
    out+=("| notes/section-${n}/item-${i}.md | mesure | ${i}0 ms | ligne ${i} du tableau ${n} |")
  done
  out+=("")
}

imageblock() {
  local n=$1 idx
  idx=$(printf '%02d' $(( n % IMAGE_COUNT )))
  out+=("![Image de référence ${n}](assets/bench-${idx}.png)" "")
}

# --- Assemblage -------------------------------------------------------------
# On boucle sur un cycle de blocs jusqu'à atteindre le nombre de lignes visé,
# plutôt que de fixer un compte par type : les variantes sont ainsi comparables
# à longueur égale, ce qui est la condition pour que la comparaison ait un sens.
write_variant() {
  local variant="$1"
  local title="Bench ${variant}"
  local path="$NOTES_DIR/bench-${variant}.md"
  local -a out=()
  local n=0

  out+=("# ${title}" "")
  out+=("Note générée par \`scripts/gen-bench-note.sh\` pour la mesure des performances." "")

  while (( ${#out[@]} < LINES )); do
    n=$(( n + 1 ))
    if (( n % 12 == 1 )); then heading "$n"; fi

    case "$variant" in
      text)
        para "$n"; wikipara "$n"; para "$n"
        (( n % 4 == 0 )) && listblock "$n"
        (( n % 6 == 0 )) && quote "$n"
        ;;
      code)    codeblock "$n" ;;
      mermaid) mermaidblock "$n" ;;
      images)  imageblock "$n" ;;
      tables)  tableblock "$n" ;;
      full)
        para "$n"
        wikipara "$n"
        (( n % 2 == 0 )) && listblock "$n"
        (( n % 3 == 0 )) && codeblock "$n"
        (( n % 5 == 0 )) && tableblock "$n"
        (( n % 7 == 0 )) && imageblock "$n"
        (( n % 11 == 0 )) && quote "$n"
        (( n % 13 == 0 )) && mermaidblock "$n"
        ;;
      *)
        echo "Variante inconnue: $variant" >&2
        exit 1
        ;;
    esac
  done

  printf '%s\n' "${out[@]}" > "$path"
  printf '  %-8s: %5d lignes, %6s  %s\n' \
    "$variant" "$(wc -l < "$path")" "$(du -h "$path" | cut -f1)" "$path"
}

echo "Coffre de mesure : $VAULT"
needs_images=0
for v in "${VARIANTS[@]}"; do
  [[ "$v" == "images" || "$v" == "full" ]] && needs_images=1
done
(( needs_images )) && gen_images

for v in "${VARIANTS[@]}"; do
  write_variant "$v"
done

cat <<EOF

Ouvrir ce coffre depuis l'application (sélecteur de coffre) pour mesurer.
Les cibles de wiki-link en « Bench Cible N » n'existent pas : c'est voulu,
la moitié des liens doit rester non résolue.
EOF
