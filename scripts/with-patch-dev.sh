#!/usr/bin/env bash
# Wrapper autour de `npm run dev` pour le watcher de `wails dev`.
# - Applique le patch models.ts avant de démarrer Vite.
# - Surveille models.ts en arrière-plan et ré-applique le patch à chaque
#   régénération par `wails dev`.
# - Lance Vite depuis le dossier frontend/.
# Wails exécute ce script depuis la racine du projet.
set -e

cd "$(dirname "$0")/.."

./scripts/patch-models.sh || echo "[with-patch] patch initial a échoué, on continue"

# Surveille les changements sur models.ts et re-patche.
(
  while true; do
    if command -v inotifywait > /dev/null 2>&1; then
      inotifywait -e close_write,moved_to frontend/wailsjs/go/models.ts > /dev/null 2>&1 || sleep 2
    else
      # Fallback : polling.
      sleep 1
    fi
    if ! grep -q "toJSON" frontend/wailsjs/go/models.ts 2>/dev/null; then
      ./scripts/patch-models.sh > /dev/null 2>&1 && echo "[with-patch] patch ré-appliqué" || true
    fi
  done
) &
WATCHER_PID=$!
trap "kill $WATCHER_PID 2>/dev/null || true" EXIT

cd frontend
exec npm run dev


