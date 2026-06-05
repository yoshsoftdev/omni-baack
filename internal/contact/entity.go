package contact

import (
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/tag"
)

type Contact struct {
	ID        uuid.UUID
	TeamID    uuid.UUID
	Name      string
	Phone     string
	Email     *string
	Notes     *string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

type Response struct {
	ID        uuid.UUID      `json:"id"`
	Name      string         `json:"name"`
	Phone     string         `json:"phone"`
	Email     *string        `json:"email,omitempty"`
	Notes     *string        `json:"notes,omitempty"`
	Tags      []tag.Response `json:"tags,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type ImportResult struct {
	Created int      `json:"created"`
	Updated int      `json:"updated"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
}

func ToResponse(c *Contact, tags []tag.Tag) Response {
	tagResponses := make([]tag.Response, 0, len(tags))
	for i := range tags {
		tagResponses = append(tagResponses, tag.ToResponse(&tags[i]))
	}
	return Response{
		ID:        c.ID,
		Name:      c.Name,
		Phone:     c.Phone,
		Email:     c.Email,
		Notes:     c.Notes,
		Tags:      tagResponses,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}
