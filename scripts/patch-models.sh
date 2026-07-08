#!/usr/bin/env bash
# Patch le fichier wailsjs/go/models.ts généré pour faire fonctionner la
# classe Time. Le générateur Wails produit une classe Time vide qui ne
# préserve pas la valeur d'origine, ce qui casse la sérialisation des
# time.Time vers Go (Go reçoit `{}` et ne peut pas le désérialiser).
#
# Ce script :
#   1. Remplace la classe Time par une version qui stocke la valeur et
#      expose toJSON() pour que JSON.stringify produise une string ISO.
#   2. Substitue toutes les références `: time.Time;` et `, time.Time`
#      par `any` / `null` dans les autres classes (compatibilité avec
#      les panneaux qui typent les champs date comme `string`).
#
# À appeler après `wails generate` ou `wails dev` / `wails build` qui
# régénèrent les bindings. Vous pouvez aussi l'appeler depuis un hook
# pre-commit.
set -euo pipefail

cd "$(dirname "$0")/.."

MODELS="frontend/wailsjs/go/models.ts"

if [ ! -f "$MODELS" ]; then
  echo "patch-models: $MODELS introuvable, rien à patcher" >&2
  exit 0
fi

# 1. Conversion des types time.Time en any (les panneaux existants
#    s'attendent à des strings, pas à des instances de Time).
sed -i 's/: time\.Time;/: any;/g; s/, time\.Time/, null/g' "$MODELS"

# 2. Patch de la classe Time pour préserver la valeur via toJSON.
python3 - <<'PY'
import re, pathlib, sys

path = pathlib.Path("frontend/wailsjs/go/models.ts")
src = path.read_text()

# Remplace le bloc complet de la classe Time.
new_block = """export class Time {
		// Patch: préserve la valeur pour que JSON.stringify donne une string ISO
		// (sinon Wails envoie `{}` que Go ne peut pas désérialiser en time.Time).
		// Ce bloc est généré par `wails generate` ; voir scripts/patch-models.sh
		// qui ré-applique le patch automatiquement.
		private _value: any;

	    static createFrom(source: any = {}) {
	        return new Time(source);
	    }

	    constructor(source: any = {}) {
	        if (source instanceof Date) source = source.toISOString();
	        if (typeof source === 'string') {
	            try { source = JSON.parse(source); } catch (_e) { /* keep as string */ }
	        }
	        this._value = source;
	    }

	    toJSON(): any {
	        if (this._value instanceof Date) return this._value.toISOString();
	        return this._value;
	    }
	}"""

pattern = re.compile(
    r"export class Time \{[\s\S]*?toJSON\(\): any \{[\s\S]*?\}\s*\}",
    re.DOTALL,
)
if pattern.search(src):
    print("patch-models: classe Time déjà patchée, rien à faire")
else:
    # Trouve la classe Time vide produite par le générateur Wails.
    fallback = re.compile(
        r"export class Time \{[\s\S]*?static createFrom[\s\S]*?constructor[\s\S]*?\n\s*\}\n\s*\}",
    )
    if fallback.search(src):
        src = fallback.sub(new_block, src, count=1)
        path.write_text(src)
        print("patch-models: classe Time patchée")
    else:
        print("patch-models: classe Time non trouvée, patch ignoré", file=sys.stderr)
PY
