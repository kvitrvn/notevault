package domain

import "time"

// Note est le modèle échangé entre le domaine Go et le frontend.
// RelativePath est toujours relatif à la racine du coffre.
type Note struct {
	RelativePath string    `json:"relativePath"`
	Title        string    `json:"title"`
	Content      string    `json:"content"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	Tags         []string  `json:"tags"`
}

type NoteSummary struct {
	RelativePath string    `json:"relativePath"`
	Title        string    `json:"title"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
