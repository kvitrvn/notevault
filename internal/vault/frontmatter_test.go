package vault

import (
	"strings"
	"testing"
	"time"

	"github.com/kvitrvn/notevault/internal/domain"
)

// TestFrontmatterPreservesUnknownKeys vérifie qu'un fichier contenant
// des clés non reconnues (aliases, status, cssclasses, owner, custom_field)
// survit à parse/serialize/parse sans perte.
func TestFrontmatterPreservesUnknownKeys(t *testing.T) {
	raw := `---
title: Projet
created: 2026-01-01T10:00:00Z
updated: 2026-01-02T10:00:00Z
tags: [project, alpha]
aliases:
  - Phoenix
  - Firebird
status: active
cssclasses:
  - wide
owner: benjamin
custom_field:
  nested: value
  count: 3
---

# Body

Content here.
`
	parsed := parse(raw)
	if parsed.Title != "Projet" {
		t.Fatalf("title: %q", parsed.Title)
	}
	if len(parsed.Tags) != 2 || parsed.Tags[0] != "project" || parsed.Tags[1] != "alpha" {
		t.Fatalf("tags: %#v", parsed.Tags)
	}
	extras := parsed.ExtraFrontmatter
	for _, k := range []string{"aliases", "status", "cssclasses", "owner", "custom_field"} {
		if _, ok := extras[k]; !ok {
			t.Fatalf("clé %q absente des extras", k)
		}
	}

	round := serialize(parsed)
	round2 := parse(round)

	if round2.Title != parsed.Title {
		t.Fatalf("title round-trip: %q vs %q", round2.Title, parsed.Title)
	}
	if len(round2.Tags) != len(parsed.Tags) {
		t.Fatalf("tags round-trip: %v vs %v", round2.Tags, parsed.Tags)
	}
	for k := range parsed.ExtraFrontmatter {
		if _, ok := round2.ExtraFrontmatter[k]; !ok {
			t.Fatalf("clé %q perdue au round-trip", k)
		}
	}
}

func TestFrontmatterRoundTripStable(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	original := domain.Note{
		RelativePath: "notes/inbox/test.md",
		Title:        "Titre",
		Content:      "Corps de la note.",
		CreatedAt:    now,
		UpdatedAt:    now,
		Tags:         []string{"alpha", "beta"},
		ExtraFrontmatter: map[string]any{
			"aliases":    []any{"Phx", "Fbr"},
			"status":     "active",
			"cssclasses": []any{"wide"},
			"owner":      "benjamin",
		},
	}

	first := serialize(original)
	if !strings.Contains(first, "aliases:") || !strings.Contains(first, "cssclasses:") {
		t.Fatalf("serialize omet les extras:\n%s", first)
	}
	second := serialize(parse(first))
	if first != second {
		t.Fatalf("round-trip non stable:\nfirst=\n%s\nsecond=\n%s", first, second)
	}
}

func TestFrontmatterWithoutFrontmatterStillParses(t *testing.T) {
	parsed := parse("hello world\n")
	if parsed.Title != "" {
		t.Fatalf("title: %q", parsed.Title)
	}
	if parsed.Content != "hello world\n" {
		t.Fatalf("content: %q", parsed.Content)
	}
}

func TestFrontmatterOpenNotClosedTreatedAsBody(t *testing.T) {
	parsed := parse("---\ntitle: broken\nincomplète\n")
	if parsed.Title != "" {
		t.Fatalf("title: %q (devrait être vide car frontmatter non fermée)", parsed.Title)
	}
	if !strings.Contains(parsed.Content, "title: broken") {
		t.Fatalf("corps: %q", parsed.Content)
	}
}

func TestSerializeProducesValidYAMLThatReparses(t *testing.T) {
	note := domain.Note{
		Title:   "Test YAML",
		Content: "body",
		Tags:    []string{"a", "b"},
		ExtraFrontmatter: map[string]any{
			"owner": "benjamin",
		},
	}
	out := serialize(note)
	parsed := parse(out)
	if parsed.Title != "Test YAML" {
		t.Fatalf("title: %q", parsed.Title)
	}
	if len(parsed.Tags) != 2 {
		t.Fatalf("tags: %v", parsed.Tags)
	}
	if parsed.ExtraFrontmatter["owner"] != "benjamin" {
		t.Fatalf("owner perdu: %#v", parsed.ExtraFrontmatter)
	}
}
