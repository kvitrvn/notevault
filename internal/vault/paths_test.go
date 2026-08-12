package vault

import (
	"path/filepath"
	"strings"
	"testing"
)

// Les quatre fonctions ci-dessous sont les points d'étranglement de la sûreté
// des chemins. Elles n'étaient couvertes que transitivement, par des tests de
// service ou HTTP : un assouplissement accidentel pouvait passer inaperçu.

func TestNormalizeAssetPath(t *testing.T) {
	valid := map[string]string{
		"assets/photo.png":            "photo.png",
		"assets/2026/07/photo.png":    filepath.Join("2026", "07", "photo.png"),
		"assets/mes photos/image.png": filepath.Join("mes photos", "image.png"),
		"  assets/photo.png  ":        "photo.png",
		"assets/./photo.png":          "photo.png",
		"assets/sous/../photo.png":    "photo.png",
	}
	for input, want := range valid {
		t.Run("ok/"+input, func(t *testing.T) {
			got, err := normalizeAssetPath(input)
			if err != nil {
				t.Fatalf("normalizeAssetPath(%q) : %v", input, err)
			}
			if got != want {
				t.Fatalf("normalizeAssetPath(%q) = %q, want %q", input, got, want)
			}
		})
	}

	invalid := []string{
		"",
		".",
		"assets",
		"assets/",
		"assets/..",
		"assets/../secret.png",
		"assets/../../etc/passwd",
		"../assets/photo.png",
		"notes/photo.png",
		"/etc/passwd",
		"/assets/photo.png",
	}
	for _, input := range invalid {
		t.Run("ko/"+input, func(t *testing.T) {
			if got, err := normalizeAssetPath(input); err == nil {
				t.Fatalf("normalizeAssetPath(%q) = %q, want une erreur", input, got)
			}
		})
	}
}

// Quelle que soit l'entrée, un chemin accepté doit rester relatif et confiné.
func FuzzNormalizeAssetPath(f *testing.F) {
	for _, seed := range []string{
		"assets/photo.png", "assets/../../etc/passwd", "", "assets/..",
		"/assets/x.png", "assets//x.png", `assets\..\..\x.png`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		got, err := normalizeAssetPath(input)
		if err != nil {
			return
		}
		if got == "" || got == "." {
			t.Fatalf("normalizeAssetPath(%q) a accepté un chemin vide", input)
		}
		if filepath.IsAbs(got) {
			t.Fatalf("normalizeAssetPath(%q) = %q : chemin absolu accepté", input, got)
		}
		if !filepath.IsLocal(got) {
			t.Fatalf("normalizeAssetPath(%q) = %q : chemin non local accepté", input, got)
		}
		for _, segment := range strings.Split(filepath.ToSlash(got), "/") {
			if segment == ".." {
				t.Fatalf("normalizeAssetPath(%q) = %q : remontée acceptée", input, got)
			}
		}
	})
}

func TestSanitizeExt(t *testing.T) {
	allowed := []string{
		"photo.png", "photo.PNG", "image.jpeg", "clip.webm", "son.mp3",
		"doc.pdf", "notes.md", "brut.txt", "anim.gif", "vect.svg",
	}
	for _, name := range allowed {
		t.Run("ok/"+name, func(t *testing.T) {
			if sanitizeExt(name) == "" {
				t.Fatalf("sanitizeExt(%q) a refusé une extension supportée", name)
			}
		})
	}

	rejected := []string{
		"script.js", "page.html", "page.htm", "binaire.exe", "lib.so",
		"archive.zip", "sansextension", "", ".", "photo.png.exe", "style.css",
	}
	for _, name := range rejected {
		t.Run("ko/"+name, func(t *testing.T) {
			if got := sanitizeExt(name); got != "" {
				t.Fatalf("sanitizeExt(%q) = %q, want \"\"", name, got)
			}
		})
	}
}

func TestNormalizeVaultRelative(t *testing.T) {
	valid := map[string]string{
		"notes/a.md":           "notes/a.md",
		"notes/sous/a.md":      "notes/sous/a.md",
		"notes/./a.md":         "notes/a.md",
		"notes/sous/../a.md":   "notes/a.md",
		".notevault/pins.json": ".notevault/pins.json",
	}
	for input, want := range valid {
		t.Run("ok/"+input, func(t *testing.T) {
			got, err := normalizeVaultRelative(input)
			if err != nil {
				t.Fatalf("normalizeVaultRelative(%q) : %v", input, err)
			}
			if got != want {
				t.Fatalf("normalizeVaultRelative(%q) = %q, want %q", input, got, want)
			}
		})
	}

	invalid := []string{"", ".", "..", "../a.md", "notes/../../a.md", "/etc/passwd", "/notes/a.md"}
	for _, input := range invalid {
		t.Run("ko/"+input, func(t *testing.T) {
			if got, err := normalizeVaultRelative(input); err == nil {
				t.Fatalf("normalizeVaultRelative(%q) = %q, want une erreur", input, got)
			}
		})
	}
}

func TestValidateNoteRelPath(t *testing.T) {
	valid := []string{"notes/a.md", "notes/sous/dossier/a.md", "notes/./a.md", "notes/x/../a.md"}
	for _, input := range valid {
		t.Run("ok/"+input, func(t *testing.T) {
			if err := validateNoteRelPath(input); err != nil {
				t.Fatalf("validateNoteRelPath(%q) : %v", input, err)
			}
		})
	}

	invalid := []string{
		"",
		".",
		"..",
		"../secret.md",
		"notes/../../secret.md",
		"/notes/a.md",
		"assets/a.md",     // hors de notes/
		"notes/a.txt",     // mauvaise extension
		"notes/a",         // pas d'extension
		"templates/a.md",  // hors de notes/
		".notevault/a.md", // hors de notes/
	}
	for _, input := range invalid {
		t.Run("ko/"+input, func(t *testing.T) {
			if err := validateNoteRelPath(input); err == nil {
				t.Fatalf("validateNoteRelPath(%q) = nil, want une erreur", input)
			}
		})
	}
}

// validateNoteRelPath et normalizeAssetPath doivent s'accorder sur le rejet
// des remontées, malgré des implémentations différentes.
func TestPathValidatorsAgreeOnTraversal(t *testing.T) {
	traversals := []string{
		"../../etc/passwd",
		"notes/../../etc/passwd",
		"assets/../../etc/passwd",
	}
	for _, input := range traversals {
		t.Run(input, func(t *testing.T) {
			if err := validateNoteRelPath(input); err == nil {
				t.Fatalf("validateNoteRelPath(%q) a accepté une remontée", input)
			}
			if _, err := normalizeAssetPath(input); err == nil {
				t.Fatalf("normalizeAssetPath(%q) a accepté une remontée", input)
			}
			if _, err := normalizeVaultRelative(input); err == nil {
				t.Fatalf("normalizeVaultRelative(%q) a accepté une remontée", input)
			}
		})
	}
}
