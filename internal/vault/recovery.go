package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// StateFile est l'état persistant de l'app (hors config utilisateur).
// Il sert à la fois pour l'onboarding (drapeau Completed) et pour la
// récupération après crash (DirtyBuffer d'une note en cours d'édition).
type StateFile struct {
	OnboardingCompleted bool        `json:"onboardingCompleted"`
	LastShownVersion    string      `json:"lastShownVersion"`
	Dirty               bool        `json:"dirty"`
	NotePath            string      `json:"notePath"`
	Buffer              string      `json:"buffer"`
	BufferCiphertext    string      `json:"bufferCiphertext,omitempty"`
	BufferSavedAt       time.Time   `json:"bufferSavedAt"`
	DiskModifiedAt      *time.Time  `json:"diskModifiedAt,omitempty"`
	Onboarding          *Onboarding `json:"onboarding,omitempty"`
}

// Onboarding mémorise les choix de l'utilisateur lors du premier lancement.
// On conserve les valeurs même après la fin de l'onboarding pour permettre
// de les restaurer depuis la page d'aide.
type Onboarding struct {
	VaultPath   string    `json:"vaultPath,omitempty"`
	Theme       string    `json:"theme,omitempty"`
	Skipped     bool      `json:"skipped"`
	CompletedAt time.Time `json:"completedAt,omitempty"`
}

const stateFileName = "state.json"

// stateStore lit/écrit StateFile depuis <root>/.notevault/state.json.
// Toute écriture passe par writeAtomic pour rester cohérent avec les
// autres fichiers internes (config.json, history/...).
type stateStore struct {
	path string
}

func newStateStore(root string) *stateStore {
	return &stateStore{path: filepath.Join(root, ".notevault", stateFileName)}
}

func (s *stateStore) Load() (StateFile, error) {
	state := StateFile{}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return state, nil
		}
		return state, fmt.Errorf("lire state.json : %w", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("decoder state.json : %w", err)
	}
	return state, nil
}

func (s *stateStore) Save(state StateFile) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("préparer le dossier : %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encoder state.json : %w", err)
	}
	if err := writeAtomic(s.path, data, 0o644); err != nil {
		return fmt.Errorf("écrire state.json : %w", err)
	}
	return nil
}

func (s *stateStore) Path() string { return s.path }

// ShouldOfferRecovery détermine si le buffer en attente correspond bien à
// une modification non sauvegardée. Conditions :
//  1. state.dirty == true
//  2. le chemin de note est renseigné et existe sur disque
//  3. la mtime du disque est antérieure au buffer (sinon, la version sur
//     disque est plus récente que le buffer — typiquement après une sync
//     externe — et on n'écrasera pas).
func ShouldOfferRecovery(state StateFile, diskMTime time.Time) bool {
	if !state.Dirty {
		return false
	}
	if state.NotePath == "" {
		return false
	}
	if state.BufferSavedAt.IsZero() {
		return false
	}
	if diskMTime.After(state.BufferSavedAt) {
		// Le disque a une version plus récente : le buffer est obsolète.
		return false
	}
	return true
}

// RecoverySnapshot est l'information présentée à l'utilisateur lorsqu'on
// propose de récupérer un buffer. Onboarding peut être nil si l'utilisateur
// n'a jamais complété l'onboarding.
type RecoverySnapshot struct {
	HasRecovery    bool        `json:"hasRecovery"`
	NotePath       string      `json:"notePath"`
	Buffer         string      `json:"buffer"`
	BufferSavedAt  time.Time   `json:"bufferSavedAt"`
	DiskModifiedAt time.Time   `json:"diskModifiedAt"`
	Onboarding     *Onboarding `json:"onboarding,omitempty"`
}
