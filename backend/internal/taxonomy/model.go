package taxonomy

import "time"

type Category struct {
	ID           uint64    `json:"id"`
	Slug         string    `json:"slug"`
	Name         string    `json:"name"`
	Enabled      bool      `json:"enabled"`
	DisplayOrder int       `json:"display_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Tag struct {
	ID             uint64 `json:"id"`
	NormalizedName string `json:"-"`
	DisplayName    string `json:"name"`
}

type VideoTaxonomy struct {
	Category *Category `json:"category,omitempty"`
	Tags     []Tag     `json:"tags"`
}

type Selection struct {
	CategoryID uint64
	Tags       []NormalizedTag
}

type NormalizedTag struct {
	Normalized string
	Display    string
}
