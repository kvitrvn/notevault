package vault

import (
	"fmt"
	"strings"
	"time"

	"github.com/kvitrvn/notevault/internal/domain"
)

func serialize(note domain.Note) string {
	tags := make([]string, 0, len(note.Tags))
	for _, tag := range note.Tags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return fmt.Sprintf("---\ntitle: %s\ncreated: %s\nupdated: %s\ntags: [%s]\n---\n\n%s\n",
		escapeYAMLScalar(note.Title),
		note.CreatedAt.UTC().Format(time.RFC3339),
		note.UpdatedAt.UTC().Format(time.RFC3339),
		strings.Join(tags, ", "),
		strings.TrimSpace(note.Content),
	)
}

// parse est volontairement minimal : les notes restent utilisables dans tout éditeur Markdown.
func parse(raw string) domain.Note {
	note := domain.Note{Tags: []string{}}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(raw, "---\n") {
		note.Content = raw
		return note
	}
	end := strings.Index(raw[4:], "\n---\n")
	if end < 0 {
		note.Content = raw
		return note
	}
	frontMatter := raw[4 : end+4]
	note.Content = strings.TrimPrefix(raw[end+9:], "\n")

	for _, line := range strings.Split(frontMatter, "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "title":
			note.Title = strings.Trim(value, "\"'")
		case "created":
			note.CreatedAt, _ = time.Parse(time.RFC3339, value)
		case "updated":
			note.UpdatedAt, _ = time.Parse(time.RFC3339, value)
		case "tags":
			value = strings.Trim(value, "[]")
			for _, tag := range strings.Split(value, ",") {
				tag = strings.Trim(strings.TrimSpace(tag), "\"'")
				if tag != "" {
					note.Tags = append(note.Tags, tag)
				}
			}
		}
	}
	return note
}

func escapeYAMLScalar(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "\n", " ")
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r >= 'à' && r <= 'ÿ':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "note"
	}
	return result
}

func template(key string) string {
	switch key {
	case "meeting":
		return "# Participants\n\n- \n\n# Ordre du jour\n\n- \n\n# Décisions\n\n- \n\n# Actions\n\n- [ ] \n"
	case "daily":
		return "# Intention\n\n\n# Journal\n\n\n# À retenir\n\n\n# Demain\n\n- [ ] \n"
	default:
		return ""
	}
}
