package vault

import (
	"context"
	"testing"
)

func TestParseFilterEmpty(t *testing.T) {
	fq, err := ParseFilter("")
	if err != nil {
		t.Fatalf("ParseFilter: %v", err)
	}
	if !fq.IsEmpty() {
		t.Fatalf("devrait être vide : %+v", fq)
	}
}

func TestParseFilterPlain(t *testing.T) {
	fq, err := ParseFilter("foo bar baz")
	if err != nil {
		t.Fatalf("ParseFilter: %v", err)
	}
	if fq.Query != "foo bar baz" {
		t.Fatalf("Query: %q", fq.Query)
	}
}

func TestParseFilterPhrase(t *testing.T) {
	fq, err := ParseFilter(`hello "exact phrase" world`)
	if err != nil {
		t.Fatalf("ParseFilter: %v", err)
	}
	want := `hello "exact phrase" world`
	if fq.Query != want {
		t.Fatalf("Query: %q (want %q)", fq.Query, want)
	}
}

func TestParseFilterTags(t *testing.T) {
	fq, err := ParseFilter("tag:projet -tag:archive foo")
	if err != nil {
		t.Fatalf("ParseFilter: %v", err)
	}
	if len(fq.Tags) != 1 || fq.Tags[0] != "projet" {
		t.Fatalf("Tags: %v", fq.Tags)
	}
	if len(fq.ExcludeTags) != 1 || fq.ExcludeTags[0] != "archive" {
		t.Fatalf("ExcludeTags: %v", fq.ExcludeTags)
	}
	if fq.Query != "foo" {
		t.Fatalf("Query: %q", fq.Query)
	}
}

func TestParseFilterPath(t *testing.T) {
	fq, err := ParseFilter("path:projects/web/*")
	if err != nil {
		t.Fatalf("ParseFilter: %v", err)
	}
	if fq.Path != "projects/web" {
		t.Fatalf("Path: %q", fq.Path)
	}
}

func TestParseFilterUpdated(t *testing.T) {
	cases := []struct {
		in       string
		fromWant bool // UpdatedFrom doit être non-zéro
		toWant   bool // UpdatedTo doit être non-zéro
	}{
		{"updated:today", true, true},
		{"updated:yesterday", true, true},
		{"updated:2026-01-15", true, true},
		{"updated:>2026-01-15", true, false},
		{"updated:<2026-01-15", false, true},
		{"updated:>=2026-01-15", true, false},
		{"updated:<=2026-01-15", false, true},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			fq, err := ParseFilter(c.in)
			if err != nil {
				t.Fatalf("ParseFilter: %v", err)
			}
			if c.fromWant && fq.UpdatedFrom.IsZero() {
				t.Fatalf("UpdatedFrom devrait être non-zéro")
			}
			if !c.fromWant && !fq.UpdatedFrom.IsZero() {
				t.Fatalf("UpdatedFrom devrait être zéro : %v", fq.UpdatedFrom)
			}
			if c.toWant && fq.UpdatedTo.IsZero() {
				t.Fatalf("UpdatedTo devrait être non-zéro")
			}
			if !c.toWant && !fq.UpdatedTo.IsZero() {
				t.Fatalf("UpdatedTo devrait être zéro : %v", fq.UpdatedTo)
			}
		})
	}
}

func TestParseFilterInvalid(t *testing.T) {
	cases := []string{
		"unknown:value",
		`"unfinished`,
		"updated:notadate",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if _, err := ParseFilter(c); err == nil {
				t.Fatalf("aurait dû échouer : %q", c)
			}
		})
	}
}

func TestParseFilterChips(t *testing.T) {
	fq, _ := ParseFilter("tag:x -tag:y path:foo tag:z")
	chips := fq.Chips()
	if len(chips) != 4 {
		t.Fatalf("chips: %d (%+v)", len(chips), chips)
	}
}

func TestRemoveChip(t *testing.T) {
	fq, _ := ParseFilter("tag:x -tag:y tag:z")
	fq2 := RemoveChip(fq, "tag", "x")
	if len(fq2.Tags) != 1 || fq2.Tags[0] != "z" {
		t.Fatalf("Tags après remove : %v", fq2.Tags)
	}
	fq3 := RemoveChip(fq2, "exclude", "-y")
	if len(fq3.ExcludeTags) != 0 {
		t.Fatalf("ExcludeTags après remove : %v", fq3.ExcludeTags)
	}
}

func TestServicePin(t *testing.T) {
	svc, _ := setupVault(t)
	n1, _ := svc.CreateNote("Pinned", "")
	n2, _ := svc.CreateNote("Other", "")

	if err := svc.Pin(n1.RelativePath, true); err != nil {
		t.Fatalf("Pin: %v", err)
	}
	pinned, err := svc.ListPinned()
	if err != nil {
		t.Fatalf("ListPinned: %v", err)
	}
	if len(pinned) != 1 || pinned[0].RelativePath != n1.RelativePath {
		t.Fatalf("ListPinned: %+v", pinned)
	}
	is, _ := svc.IsPinned(n2.RelativePath)
	if is {
		t.Fatal("n2 ne devrait pas être épinglée")
	}
	if err := svc.Pin(n1.RelativePath, false); err != nil {
		t.Fatalf("Unpin: %v", err)
	}
	pinned, _ = svc.ListPinned()
	if len(pinned) != 0 {
		t.Fatalf("ListPinned après unpin: %d", len(pinned))
	}
}

func TestServiceListNotesFiltered(t *testing.T) {
	svc, _ := setupVault(t)
	a, _ := svc.CreateNote("Alpha", "")
	a.Tags = []string{"projet", "important"}
	if _, err := svc.SaveNote(a); err != nil {
		t.Fatalf("SaveNote a: %v", err)
	}
	b, _ := svc.CreateNote("Beta", "")
	b.Tags = []string{"archive"}
	if _, err := svc.SaveNote(b); err != nil {
		t.Fatalf("SaveNote b: %v", err)
	}
	if err := svc.IndexNow(context.Background(), nil); err != nil {
		t.Fatalf("IndexNow: %v", err)
	}

	got, err := svc.ListNotesFiltered(FilterQuery{Tags: []string{"projet"}}, 0)
	if err != nil {
		t.Fatalf("ListNotesFiltered: %v", err)
	}
	if len(got) != 1 || got[0].RelativePath != a.RelativePath {
		t.Fatalf("filtre tag: %+v", got)
	}
	got, _ = svc.ListNotesFiltered(FilterQuery{ExcludeTags: []string{"archive"}}, 0)
	if len(got) != 1 || got[0].RelativePath != a.RelativePath {
		t.Fatalf("filtre exclude: %+v", got)
	}
	got, _ = svc.ListNotesFiltered(FilterQuery{Tags: []string{"projet", "important"}}, 0)
	if len(got) != 1 {
		t.Fatalf("filtre 2 tags: %+v", got)
	}
	got, _ = svc.ListNotesFiltered(FilterQuery{Query: "Alpha"}, 0)
	if len(got) != 1 || got[0].RelativePath != a.RelativePath {
		t.Fatalf("filtre query: %+v", got)
	}
}

func TestServiceOpenDailyNoteAuto(t *testing.T) {
	svc, _ := setupVault(t)
	cfg, _ := svc.GetConfig()
	cfg.AutoDailyNote = true
	if err := svc.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	rel, err := svc.EnsureDailyNote()
	if err != nil {
		t.Fatalf("EnsureDailyNote: %v", err)
	}
	if rel == "" {
		t.Fatal("EnsureDailyNote aurait dû retourner un chemin")
	}
	rel2, err := svc.EnsureDailyNote()
	if err != nil {
		t.Fatalf("EnsureDailyNote 2: %v", err)
	}
	if rel2 != rel {
		t.Fatalf("chemin différent : %s vs %s", rel, rel2)
	}
	note, err := svc.OpenDailyNote()
	if err != nil {
		t.Fatalf("OpenDailyNote: %v", err)
	}
	if note.Title == "" {
		t.Fatal("titre vide")
	}
}

func TestServiceListFolders(t *testing.T) {
	svc, _ := setupVault(t)
	for _, title := range []string{"A", "B", "C"} {
		if _, err := svc.CreateNote(title, ""); err != nil {
			t.Fatalf("CreateNote: %v", err)
		}
	}
	if err := svc.IndexNow(context.Background(), nil); err != nil {
		t.Fatalf("IndexNow: %v", err)
	}
	folders, err := svc.ListFolders()
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	found := false
	for _, f := range folders {
		if f.Path == "inbox" {
			found = true
			if f.Count < 1 {
				t.Fatalf("Count: %d", f.Count)
			}
		}
	}
	if !found {
		t.Fatalf("dossier inbox manquant : %+v", folders)
	}
}

func TestSanitizeFilter(t *testing.T) {
	// Smoke : doit gérer des inputs tordus sans paniquer.
	fq, err := ParseFilter(`foo "phrase complète" tag:dedans -tag:archive`)
	if err != nil {
		t.Fatalf("ParseFilter: %v", err)
	}
	if fq.Query != `foo "phrase complète"` {
		t.Fatalf("Query: %q", fq.Query)
	}
	if len(fq.Tags) != 1 || fq.Tags[0] != "dedans" {
		t.Fatalf("Tags: %v", fq.Tags)
	}
	if len(fq.ExcludeTags) != 1 || fq.ExcludeTags[0] != "archive" {
		t.Fatalf("ExcludeTags: %v", fq.ExcludeTags)
	}
}
