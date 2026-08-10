package vault

import (
	"context"
	"testing"
)

func TestListNoteSummariesByPaths(t *testing.T) {
	svc, _ := setupVault(t)
	a, _ := svc.CreateNote("", "Alpha", "")
	b, _ := svc.CreateNote("", "Beta", "")
	if err := svc.IndexNow(context.Background(), nil); err != nil {
		t.Fatalf("IndexNow: %v", err)
	}
	got, err := svc.ListNoteSummariesByPaths([]string{a.RelativePath, "notes/missing.md", b.RelativePath})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("attendu 2, obtenu %d", len(got))
	}
	if got[0].Title == "" || got[1].Title == "" {
		t.Fatalf("résumés vides : %#v", got)
	}
}
