package chat

import (
	"bytes"
	"testing"

	goanon "github.com/bornholm/go-anon"
	goanonModel "github.com/bornholm/go-anon/pkg/model"
)

// TestLoadModelReadsStreamFormat verrouille la compatibilité entre la version
// épinglée de go-anon et le format de modèle publié par le manifeste amont.
//
// Le manifeste sert aujourd'hui des modèles au format flux (magic « GOANONv4 »)
// tout en gardant le même numéro de schéma : une bibliothèque trop ancienne les
// télécharge, valide leur empreinte, puis échoue seulement au chargement avec
// « gob: duplicate type received ». Ce test fait échouer la suite plutôt que le
// panneau de discussion de l'utilisateur.
func TestLoadModelReadsStreamFormat(t *testing.T) {
	t.Parallel()

	source := &goanonModel.CRF{
		Labels:     []string{"O", "B-PER"},
		LabelIndex: map[string]int{"O": 0, "B-PER": 1},
		Weights: &goanonModel.SparseWeights{
			Keys: []uint64{1, 2},
			Vals: []float32{0.25, -0.5},
		},
		Transition: [][]float64{{0, 0}, {0, 0}},
	}

	var buffer bytes.Buffer
	if err := source.SaveStream(&buffer); err != nil {
		t.Fatalf("SaveStream : %v", err)
	}
	if !bytes.Contains(buffer.Bytes()[:2], []byte{0x1f, 0x8b}) {
		t.Fatal("le flux sérialisé n'est pas gzippé")
	}

	// Voie utilisée par l'application (privacy.go).
	loaded, err := goanon.LoadModel(bytes.NewReader(buffer.Bytes()))
	if err != nil {
		t.Fatalf("LoadModel refuse le format flux : %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadModel a retourné un modèle nul")
	}

	// Le CRF sous-jacent est opaque derrière ner.Model : on revérifie le contenu
	// via le décodeur de bas niveau.
	decoded, err := goanonModel.LoadModel(bytes.NewReader(buffer.Bytes()))
	if err != nil {
		t.Fatalf("model.LoadModel refuse le format flux : %v", err)
	}
	if got := len(decoded.Labels); got != len(source.Labels) {
		t.Fatalf("labels chargés = %d, attendu %d", got, len(source.Labels))
	}
}
