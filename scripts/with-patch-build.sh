#!/usr/bin/env bash
# Wrapper autour de `npm run build` pour le build de `wails build`.
# Applique le patch models.ts avant la compilation Vite. Wails exécute
# ce script depuis la racine du projet.
set -e

cd "$(dirname "$0")/.."

./scripts/patch-models.sh

cd frontend
exec npm run build

