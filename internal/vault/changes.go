package vault

import "sync"

// ChangeKind qualifie l'opération réellement appliquée à l'index après
// observation du système de fichiers. Le watcher peut émettre plusieurs
// observations pour un même chemin ; le service déduplique et choisit
// un Kind final par chemin avant de publier le batch.
type ChangeKind string

const (
	ChangeUpsert ChangeKind = "upsert"
	ChangeDelete ChangeKind = "delete"
)

// VaultChange décrit la mutation d'une note telle que constatée par
// l'index après traitement. Path est relatif à la racine du coffre
// (forme slash, ex. "notes/inbox/note.md").
type VaultChange struct {
	Kind ChangeKind `json:"kind"`
	Path string     `json:"path"`
}

// VaultChangeBatch regroupe les changements d'un même flush du watcher
// (ou d'un rescan complet). Revision est monotone pour permettre au
// frontend d'ignorer un batch si une action utilisateur arrive
// simultanément. FullRescan signifie que le frontend doit invalider
// l'intégralité de son cache et reconstruire depuis l'API.
type VaultChangeBatch struct {
	Revision   uint64        `json:"revision"`
	Changes    []VaultChange `json:"changes"`
	FullRescan bool          `json:"fullRescan"`
}

// ChangeHandler reçoit un batch déjà appliqué à l'index. Les abonnés
// ne doivent pas bloquer longtemps — la publication est sérialisée par
// le service mais le watcher reprend sa boucle immédiatement après.
type ChangeHandler func(batch VaultChangeBatch)

// changeBus distribue les batches aux abonnés du Service. Il est interne
// pour découpler le watcher de Wails et permettre plusieurs abonnés
// (tests, diagnostics, UI).
type changeBus struct {
	mu       sync.RWMutex
	handlers []ChangeHandler
	revision uint64
	muRev    sync.Mutex
}

func newChangeBus() *changeBus {
	return &changeBus{}
}

func (b *changeBus) Subscribe(h ChangeHandler) {
	if h == nil {
		return
	}
	b.mu.Lock()
	b.handlers = append(b.handlers, h)
	b.mu.Unlock()
}

func (b *changeBus) Publish(batch VaultChangeBatch) {
	if len(batch.Changes) == 0 && !batch.FullRescan {
		return
	}
	b.muRev.Lock()
	b.revision++
	batch.Revision = b.revision
	b.muRev.Unlock()

	b.mu.RLock()
	handlers := make([]ChangeHandler, len(b.handlers))
	copy(handlers, b.handlers)
	b.mu.RUnlock()

	for _, h := range handlers {
		h(batch)
	}
}
