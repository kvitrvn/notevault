package vault

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kvitrvn/notevault/internal/domain"
)

// TestListNotesReturnsAllTenThousand crée 10 000 notes, indexe le
// service et vérifie que ListNotes() en retourne bien 10 000. Couvre
// la régression décrite par l'audit : un cap de 5000 coupait
// silencieusement la moitié d'un vault cible.
func TestListNotesReturnsAllTenThousand(t *testing.T) {
	if testing.Short() {
		t.Skip("lent : crée 10 000 notes")
	}
	svc, dir := setupVault(t)
	const n = 10_000
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		folder := fmt.Sprintf("notes/folder-%03d", i%100)
		rel := fmt.Sprintf("%s/note-%05d.md", folder, i)
		path := dir + "/" + rel
		note := domain.Note{
			RelativePath: rel,
			Title:        fmt.Sprintf("Note %05d", i),
			Content:      "# Title\n\nbody",
			CreatedAt:    now,
			UpdatedAt:    now.Add(time.Duration(i) * time.Second),
		}
		if err := writeFile(path, serialize(note)); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	if err := svc.IndexNow(context.Background(), nil); err != nil {
		t.Fatalf("IndexNow: %v", err)
	}
	notes, err := svc.ListNotes()
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if got := len(notes); got != n {
		t.Fatalf("ListNotes: %d notes, attendu %d", got, n)
	}
}

func TestListNotesFilteredHonorsExplicitZero(t *testing.T) {
	svc, dir := setupVault(t)
	for i := 0; i < 50; i++ {
		path := dir + fmt.Sprintf("/notes/inbox/note-%03d.md", i)
		if err := writeFile(path, fmt.Sprintf("---\ntitle: Note %03d\n---\n\nbody", i)); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	if err := svc.IndexNow(context.Background(), nil); err != nil {
		t.Fatalf("IndexNow: %v", err)
	}
	got, err := svc.ListNotesFiltered(FilterQuery{}, 0)
	if err != nil {
		t.Fatalf("ListNotesFiltered: %v", err)
	}
	if len(got) != 50 {
		t.Fatalf("attendu 50, obtenu %d", len(got))
	}
}
