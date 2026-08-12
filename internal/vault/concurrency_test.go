package vault

import (
	"strings"
	"sync"
	"testing"
)

// Deux SaveNote simultanés sur la même note entrelaçaient l'instantané
// d'historique, la rotation et l'écriture. Le verrou par chemin les sérialise.
func TestConcurrentSaveNoteSamePath(t *testing.T) {
	svc, _ := setupVault(t)
	note, err := svc.CreateNote("", "Concurrence", "")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	const writers = 8
	const rounds = 10
	var wg sync.WaitGroup
	errs := make(chan error, writers*rounds)
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				candidate := note
				candidate.Content = strings.Repeat("x", id+1)
				if _, err := svc.SaveNote(candidate); err != nil {
					errs <- err
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("SaveNote concurrent : %v", err)
	}

	// La note doit rester lisible et cohérente après la rafale.
	saved, err := svc.OpenNote(note.RelativePath)
	if err != nil {
		t.Fatalf("OpenNote après rafale : %v", err)
	}
	if saved.Title == "" {
		t.Fatal("note corrompue après écritures concurrentes")
	}
}

// Lectures et écritures mélangées : cible les verrous de l'index et le cache
// de configuration ajoutés sur le chemin chaud de sauvegarde.
func TestConcurrentReadsDuringSaves(t *testing.T) {
	svc, _ := setupVault(t)
	for i := 0; i < 20; i++ {
		if _, err := svc.CreateNote("", "Note "+itoa(i), ""); err != nil {
			t.Fatalf("CreateNote: %v", err)
		}
	}
	note, err := svc.CreateNote("", "Cible", "")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	errs := make(chan error, 64)

	reader := func(fn func() error) {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if err := fn(); err != nil {
				errs <- err
				return
			}
		}
	}

	wg.Add(4)
	go reader(func() error { _, err := svc.ListNotes(); return err })
	go reader(func() error { _, err := svc.ListPinned(); return err })
	go reader(func() error { _, err := svc.Search("note", 20); return err })
	go reader(func() error { _, err := svc.ListFolders(); return err })

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 40; i++ {
			candidate := note
			candidate.Content = "révision " + itoa(i)
			if _, err := svc.SaveNote(candidate); err != nil {
				errs <- err
				break
			}
			if err := svc.Pin(note.RelativePath, i%2 == 0); err != nil {
				errs <- err
				break
			}
		}
		close(stop)
	}()

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("opération concurrente : %v", err)
	}
}
