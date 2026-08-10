package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/kvitrvn/notevault/internal/domain"
)

// ErrConflict signale que la note sur disque a été modifiée depuis sa
// dernière lecture. L'appelant peut soit relire la note, soit forcer
// l'écrasement via SaveNoteForce.
var ErrConflict = errors.New("conflit d'édition : la note a été modifiée sur disque")

// NoteFile représente une note conjointement avec une révision disque
// (sha256 hexadécimal du contenu brut). La révision est utilisée par
// SaveNote pour détecter une modification externe concurrente.
type NoteFile struct {
	Note     domain.Note `json:"note"`
	Revision string      `json:"revision"`
}

func revisionOf(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
