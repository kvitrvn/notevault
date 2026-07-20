package vault

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestThemeLoaderBuiltin(t *testing.T) {
	loader := NewThemeLoader(t.TempDir())
	themes := loader.List()
	if len(themes) < 2 {
		t.Fatalf("themes builtin : %d (attendu ≥ 2)", len(themes))
	}
	seen := make(map[string]bool, len(themes))
	for _, th := range themes {
		seen[th.ID] = true
	}
	if !seen["dark"] || !seen["light"] {
		t.Fatalf("dark/light manquants : %v", seen)
	}
}

func TestThemeLoaderCustom(t *testing.T) {
	dir := t.TempDir()
	themesDir := filepath.Join(dir, ".notevault", "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	raw := map[string]interface{}{
		"name": "Sépia",
		"vars": map[string]string{
			"--color-accent":   "#c8a96a",
			"--color-foreground": "#3a2e1f",
		},
	}
	data, _ := json.Marshal(raw)
	if err := os.WriteFile(filepath.Join(themesDir, "sepia.json"), data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	loader := NewThemeLoader(dir)
	themes := loader.List()
	var found *Theme
	for i, th := range themes {
		if th.ID == "sepia" || th.Name == "Sépia" {
			found = &themes[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("thème sépia absent : %+v", themes)
	}
	if found.Vars["--color-accent"] != "#c8a96a" {
		t.Fatalf("var accent : %q", found.Vars["--color-accent"])
	}
	if found.Builtin {
		t.Fatal("sepia ne doit pas être builtin")
	}
}

func TestThemeLoaderCustomOverridesBuiltin(t *testing.T) {
	dir := t.TempDir()
	themesDir := filepath.Join(dir, ".notevault", "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	raw := `{"id":"dark","name":"Sombre perso","vars":{"--color-accent":"#ff0000"}}`
	if err := os.WriteFile(filepath.Join(themesDir, "dark-override.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	loader := NewThemeLoader(dir)
	th, err := loader.Get("dark")
	if err != nil {
		t.Fatalf("Get dark: %v", err)
	}
	if th.Name != "Sombre perso" {
		t.Fatalf("override non appliqué : %q", th.Name)
	}
	if th.Vars["--color-accent"] != "#ff0000" {
		t.Fatalf("var non surchargée : %q", th.Vars["--color-accent"])
	}
}

func TestThemeLoaderRejectsNonCSSVars(t *testing.T) {
	dir := t.TempDir()
	themesDir := filepath.Join(dir, ".notevault", "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	raw := `{"id":"weird","name":"X","vars":{"color-accent":"#ff0000","--color-ok":"#0f0"}}`
	if err := os.WriteFile(filepath.Join(themesDir, "weird.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	loader := NewThemeLoader(dir)
	th, err := loader.Get("weird")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, present := th.Vars["color-accent"]; present {
		t.Fatal("var sans préfixe -- acceptée")
	}
	if th.Vars["--color-ok"] != "#0f0" {
		t.Fatal("var valide perdue")
	}
}

func TestServiceListThemesIncludesBuiltins(t *testing.T) {
	svc, _ := setupVault(t)
	themes := svc.ListThemes()
	if len(themes) < 2 {
		t.Fatalf("themes via service : %d", len(themes))
	}
	th, err := svc.Theme("light")
	if err != nil {
		t.Fatalf("Theme light: %v", err)
	}
	if th.ID != "light" {
		t.Fatalf("ID : %q", th.ID)
	}
}

func TestExportNotesBasic(t *testing.T) {
	svc, dir := setupVault(t)
	note, err := svc.CreateNote("Exportable", "meeting")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	note.Content = "Mise à jour du contenu"
	if _, err := svc.SaveNote(note); err != nil {
		t.Fatalf("SaveNote: %v", err)
	}

	dest := filepath.Join(dir, "export.zip")
	if err := svc.ExportNotes([]string{note.RelativePath}, dest); err != nil {
		t.Fatalf("ExportNotes: %v", err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("zip absent : %v", err)
	}

	zr, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	if len(zr.File) == 0 {
		t.Fatal("zip vide")
	}
	found := false
	for _, f := range zr.File {
		if f.Name == note.RelativePath {
			found = true
			rc, _ := f.Open()
			defer rc.Close()
			data, _ := readAll(rc)
			if !bytes.Contains(data, []byte("Mise à jour du contenu")) {
				t.Fatalf("contenu manquant dans le zip : %q", string(data))
			}
		}
	}
	if !found {
		t.Fatalf("note %s absente du zip : %+v", note.RelativePath, zr.File)
	}
}

func TestExportNotesWithAssets(t *testing.T) {
	svc, dir := setupVault(t)
	note, err := svc.CreateNote("Avec image", "")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	relAsset, err := svc.SaveAsset([]byte("PNG-BYTES"), "logo.png")
	if err != nil {
		t.Fatalf("SaveAsset: %v", err)
	}
	note.Content = "![logo](" + relAsset + ")\n"
	if _, err := svc.SaveNote(note); err != nil {
		t.Fatalf("SaveNote: %v", err)
	}
	dest := filepath.Join(dir, "with-assets.zip")
	if err := svc.ExportNotes([]string{note.RelativePath}, dest); err != nil {
		t.Fatalf("ExportNotes: %v", err)
	}
	zr, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	var noteEntry, assetEntry bool
	for _, f := range zr.File {
		if f.Name == note.RelativePath {
			noteEntry = true
		}
		if f.Name == relAsset {
			assetEntry = true
		}
	}
	if !noteEntry {
		t.Fatal("note absente du zip")
	}
	if !assetEntry {
		t.Fatal("asset référencé absent du zip")
	}
}

func TestExportRejectsForgedAssetPath(t *testing.T) {
	svc, dir := setupVault(t)
	parent := filepath.Dir(dir)
	bait := filepath.Join(parent, "notevault-evil-export.txt")
	sentinel := []byte("BAIT-DO-NOT-EXFILTRATE")
	if err := os.WriteFile(bait, sentinel, 0o644); err != nil {
		t.Fatalf("write bait: %v", err)
	}
	defer os.Remove(bait)

	cases := []struct {
		name    string
		forged  string // chemin d'asset tel qu'il apparaîtrait dans le Markdown
	}{
		{
			name:   "traversal parent",
			forged: "assets/../../notevault-evil-export.txt",
		},
		{
			name:   "traversal profond",
			forged: "assets/2026/07/../../../../../notevault-evil-export.txt",
		},
		{
			name:   "chemin absolu",
			forged: "assets/" + filepath.ToSlash(bait),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			note, err := svc.CreateNote("Exportable", "")
			if err != nil {
				t.Fatalf("CreateNote: %v", err)
			}
			note.Content = "![evil](" + tc.forged + ")\n"
			if _, err := svc.SaveNote(note); err != nil {
				t.Fatalf("SaveNote: %v", err)
			}

			dest := filepath.Join(dir, tc.name+".zip")
			if err := svc.ExportNotes([]string{note.RelativePath}, dest); err != nil {
				t.Fatalf("ExportNotes: %v", err)
			}
			zr, err := zip.OpenReader(dest)
			if err != nil {
				t.Fatalf("open zip: %v", err)
			}
			defer zr.Close()

			for _, f := range zr.File {
				if f.Name != note.RelativePath {
					t.Fatalf("entrée inattendue dans le zip : %s", f.Name)
				}
				rc, _ := f.Open()
				data, _ := readAll(rc)
				rc.Close()
				if bytes.Contains(data, sentinel) {
					t.Fatalf("contenu exfiltré dans %s : %q", f.Name, data)
				}
			}
		})
	}
}

func TestExportNotesByTitle(t *testing.T) {
	svc, dir := setupVault(t)
	note, _ := svc.CreateNote("Titre exact", "")
	dest := filepath.Join(dir, "title.zip")
	if err := svc.ExportNotes([]string{"Titre exact"}, dest); err != nil {
		t.Fatalf("ExportNotes: %v", err)
	}
	zr, _ := zip.OpenReader(dest)
	defer zr.Close()
	found := false
	for _, f := range zr.File {
		if f.Name == note.RelativePath {
			found = true
		}
	}
	if !found {
		t.Fatal("résolution par titre échouée")
	}
}

func TestExportNotesRejectsUnknown(t *testing.T) {
	svc, dir := setupVault(t)
	dest := filepath.Join(dir, "x.zip")
	if err := svc.ExportNotes([]string{"introuvable"}, dest); err == nil {
		t.Fatal("aurait dû échouer sur un chemin inconnu")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatal("zip créé malgré l'échec")
	}
}

func TestExportNotesEmpty(t *testing.T) {
	svc, dir := setupVault(t)
	dest := filepath.Join(dir, "x.zip")
	if err := svc.ExportNotes(nil, dest); err == nil {
		t.Fatal("aurait dû échouer sur liste vide")
	}
}

func TestCountWordsIgnoresCodeAndFrontmatter(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"hello world", 2},
		{"# Titre\n\ndu texte ici", 4},
		{"---\ntitle: x\n---\n\ncontenu visible", 2},
		{"```go\nfunc x() {}\n```\naprès", 1},
		{"**gras** et *italique*", 3},
	}
	for _, c := range cases {
		got := countWords(c.in)
		if got != c.want {
			t.Errorf("countWords(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestStatsEmpty(t *testing.T) {
	svc, _ := setupVault(t)
	stats, err := svc.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalNotes != 0 || stats.TotalWords != 0 {
		t.Fatalf("stats non vides : %+v", stats)
	}
	if stats.WindowDays != 30 {
		t.Fatalf("WindowDays : %d", stats.WindowDays)
	}
	if len(stats.CreatedByDay) != 0 || len(stats.ModifiedByDay) != 0 {
		t.Fatalf("buckets non vides : %+v", stats)
	}
}

func TestStatsWithNotes(t *testing.T) {
	svc, _ := setupVault(t)
	notes := []struct {
		title string
		body  string
		tags  []string
	}{
		{"alpha", "lorem ipsum dolor", []string{"x"}},
		{"beta", "lorem et dolor", []string{"x", "y"}},
		{"gamma", "lorem seul", []string{"y"}},
	}
	for _, n := range notes {
		created, err := svc.CreateNote(n.title, "")
		if err != nil {
			t.Fatalf("CreateNote: %v", err)
		}
		created.Content = n.body
		created.Tags = n.tags
		if _, err := svc.SaveNote(created); err != nil {
			t.Fatalf("SaveNote: %v", err)
		}
	}
	if err := svc.IndexNow(context.Background(), nil); err != nil {
		t.Fatalf("IndexNow: %v", err)
	}
	stats, err := svc.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalNotes != 3 {
		t.Fatalf("TotalNotes: %d", stats.TotalNotes)
	}
	if stats.TotalWords == 0 {
		t.Fatal("TotalWords = 0")
	}
	if len(stats.TopTags) == 0 {
		t.Fatal("TopTags vide")
	}
	// tags : "x" → 2 occurrences, "y" → 2 → ordre arbitraire.
	foundX := false
	foundY := false
	for _, t := range stats.TopTags {
		if t.Tag == "x" {
			foundX = true
		}
		if t.Tag == "y" {
			foundY = true
		}
	}
	if !foundX || !foundY {
		t.Fatalf("tags manquants : %+v", stats.TopTags)
	}
}

func TestStatsBucketsAlignedOnWindow(t *testing.T) {
	svc, _ := setupVault(t)
	// Crée une note aujourd'hui.
	note, _ := svc.CreateNote("Today", "")
	if _, err := svc.SaveNote(note); err != nil {
		t.Fatalf("SaveNote: %v", err)
	}
	if err := svc.IndexNow(context.Background(), nil); err != nil {
		t.Fatalf("IndexNow: %v", err)
	}
	buckets, err := svc.index.StatsBuckets(30)
	if err != nil {
		t.Fatalf("StatsBuckets: %v", err)
	}
	today := nowUTC().Format("2006-01-02")
	found := false
	for _, d := range buckets.Created {
		if d.Day == today {
			found = d.Count >= 1
		}
	}
	if !found {
		t.Fatalf("note du jour absente des buckets : %+v", buckets.Created)
	}
}

func TestStatsWindowEdge(t *testing.T) {
	svc, _ := setupVault(t)
	buckets, err := svc.index.StatsBuckets(7)
	if err != nil {
		t.Fatalf("StatsBuckets: %v", err)
	}
	if buckets.Created == nil {
		t.Fatal("Created nil")
	}
}

func TestStatsAssetsSize(t *testing.T) {
	svc, _ := setupVault(t)
	_, err := svc.SaveAsset([]byte("abcdefghij"), "test.txt")
	if err != nil {
		t.Fatalf("SaveAsset: %v", err)
	}
	stats, err := svc.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalAssets != 10 {
		t.Fatalf("TotalAssets: %d (attendu 10)", stats.TotalAssets)
	}
}

func TestStateStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := newStateStore(dir)
	state := StateFile{
		OnboardingCompleted: true,
		Dirty:               true,
		NotePath:            "notes/inbox/x.md",
		Buffer:              "contenu non sauvegardé",
		BufferSavedAt:       nowUTC(),
		Onboarding: &Onboarding{
			Theme:       "dark",
			CompletedAt: nowUTC(),
		},
	}
	if err := store.Save(state); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.OnboardingCompleted {
		t.Fatal("OnboardingCompleted perdu")
	}
	if !loaded.Dirty || loaded.NotePath != state.NotePath {
		t.Fatalf("dirty perdu : %+v", loaded)
	}
	if loaded.Buffer != state.Buffer {
		t.Fatalf("buffer perdu : %q vs %q", loaded.Buffer, state.Buffer)
	}
	if loaded.Onboarding == nil || loaded.Onboarding.Theme != "dark" {
		t.Fatalf("onboarding perdu : %+v", loaded.Onboarding)
	}
}

func TestStateStoreLoadWhenMissing(t *testing.T) {
	store := newStateStore(t.TempDir())
	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.OnboardingCompleted || state.Dirty {
		t.Fatalf("état par défaut non vide : %+v", state)
	}
}

func TestShouldOfferRecoveryRules(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name  string
		state StateFile
		mtime time.Time
		want  bool
	}{
		{
			name:  "dirty + chemin + mtime ancienne",
			state: StateFile{Dirty: true, NotePath: "x", BufferSavedAt: now},
			mtime: now.Add(-time.Minute),
			want:  true,
		},
		{
			name:  "non dirty",
			state: StateFile{Dirty: false, NotePath: "x", BufferSavedAt: now},
			mtime: now.Add(-time.Minute),
			want:  false,
		},
		{
			name:  "chemin vide",
			state: StateFile{Dirty: true, NotePath: "", BufferSavedAt: now},
			mtime: now.Add(-time.Minute),
			want:  false,
		},
		{
			name:  "disque plus récent que buffer",
			state: StateFile{Dirty: true, NotePath: "x", BufferSavedAt: now.Add(-time.Hour)},
			mtime: now,
			want:  false,
		},
		{
			name:  "bufferSavedAt zéro",
			state: StateFile{Dirty: true, NotePath: "x", BufferSavedAt: time.Time{}},
			mtime: now,
			want:  false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ShouldOfferRecovery(c.state, c.mtime)
			if got != c.want {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestServiceSnapshotForStartupFresh(t *testing.T) {
	svc, _ := setupVault(t)
	snap, err := svc.SnapshotForStartup()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snap.HasRecovery {
		t.Fatal("HasRecovery true à froid")
	}
	if snap.Onboarding != nil {
		t.Fatalf("Onboarding non nil : %+v", snap.Onboarding)
	}
}

func TestServiceSetAndClearDirtyBuffer(t *testing.T) {
	svc, _ := setupVault(t)
	if err := svc.SetDirtyBuffer("notes/inbox/x.md", "buffer", time.Time{}); err != nil {
		t.Fatalf("SetDirtyBuffer: %v", err)
	}
	if err := svc.ClearDirtyBuffer(); err != nil {
		t.Fatalf("ClearDirtyBuffer: %v", err)
	}
	state, _ := svc.LoadState()
	if state.Dirty {
		t.Fatal("dirty non effacé")
	}
	if state.Buffer != "" {
		t.Fatalf("buffer non vidé : %q", state.Buffer)
	}
}

func TestServiceOnboardingFlag(t *testing.T) {
	svc, _ := setupVault(t)
	ob := &Onboarding{Theme: "light"}
	if err := svc.MarkOnboardingCompleted(ob); err != nil {
		t.Fatalf("Mark: %v", err)
	}
	state, _ := svc.LoadState()
	if !state.OnboardingCompleted {
		t.Fatal("drapeau non persisté")
	}
	if state.Onboarding == nil || state.Onboarding.Theme != "light" {
		t.Fatalf("onboarding absent ou incomplet : %+v", state.Onboarding)
	}
}

func TestServiceSnapshotOffersRecoveryBeforeOnboarding(t *testing.T) {
	svc, _ := setupVault(t)
	// Crée une vraie note sur disque pour que fileModified réussisse.
	note, _ := svc.CreateNote("Test", "")
	if err := svc.SetDirtyBuffer(note.RelativePath, "contenu en attente", time.Time{}); err != nil {
		t.Fatalf("SetDirtyBuffer: %v", err)
	}
	snap, err := svc.SnapshotForStartup()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if !snap.HasRecovery {
		t.Fatalf("recovery attendue : %+v", snap)
	}
	if snap.NotePath != note.RelativePath {
		t.Fatalf("NotePath : %q", snap.NotePath)
	}
	if snap.Buffer != "contenu en attente" {
		t.Fatalf("Buffer : %q", snap.Buffer)
	}
}

func TestReferencedAssets(t *testing.T) {
	cases := []struct {
		name string
		md   string
		want []string
	}{
		{"vide", "", nil},
		{"simple", "![alt](assets/2026/07/abc.png)", []string{"assets/2026/07/abc.png"}},
		{
			"multiples",
			"![a](assets/2026/07/abc.png)\ntexte\n![b](assets/2026/07/def.jpg)",
			[]string{"assets/2026/07/abc.png", "assets/2026/07/def.jpg"},
		},
		{
			"ignore http",
			"![a](https://example.com/x.png)\n![b](assets/2026/07/x.png)",
			[]string{"assets/2026/07/x.png"},
		},
		{
			"ignore non-assets",
			"![a](notes/inbox/y.png)",
			nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := referencedAssets(c.md)
			if !stringSlicesEqual(got, c.want) {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func readAll(rc interface {
	Read(p []byte) (n int, err error)
}) ([]byte, error) {
	var buf bytes.Buffer
	tmp := make([]byte, 1024)
	for {
		n, err := rc.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if err != nil {
			if err.Error() == "EOF" {
				return buf.Bytes(), nil
			}
			return buf.Bytes(), err
		}
	}
}
