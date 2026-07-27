package template

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Status is the domain approval state of a WhatsApp template.
// Values align with stored enum template_status; they are not Graph API DTOs.
type Status string

const (
	StatusApproved Status = "APPROVED"
	StatusPending  Status = "PENDING"
	StatusRejected Status = "REJECTED"
)

// Category is the domain template category (DB enum template_category).
type Category string

const (
	CategoryMarketing      Category = "MARKETING"
	CategoryUtility        Category = "UTILITY"
	CategoryAuthentication Category = "AUTHENTICATION"
)

// Template is the domain source of truth for a WhatsApp message template.
//
// Boundaries:
//   - This type MUST NOT import or embed Meta Graph API DTOs.
//   - Meta ↔ domain conversion belongs in the mapper (Phase 3).
//   - Consumers (Campaign, Inbox, API) use Template via Repository / services only.
//
// WorkspaceID is TeamID in this codebase (multi-tenant workspace).
type Template struct {
	ID             uuid.UUID `json:"id"`
	ChannelID      uuid.UUID `json:"channel_id"`
	TeamID         uuid.UUID `json:"team_id,omitempty"` // WorkspaceID
	WABAID         string    `json:"waba_id"`
	MetaTemplateID string    `json:"meta_template_id"`
	Name           string    `json:"name"`
	Language       string    `json:"language"`
	// NormalizedLanguage is a stable locale key for filter/search (Phase 5 fills via LanguageResolver).
	NormalizedLanguage string  `json:"normalized_language,omitempty"`
	Category           *string `json:"category,omitempty"`
	Status             Status  `json:"status"`
	Preview            *string `json:"preview,omitempty"`
	// Components is opaque domain JSON (structure owned by Template domain).
	// It is not a Meta MessageTemplate DTO; mappers alone interpret Graph shapes.
	Components    json.RawMessage `json:"components,omitempty"`
	VariableCount int             `json:"variable_count,omitempty"`
	// Metadata holds internal fields that must survive Meta sync (e.g. storage_object_id).
	// Never sent to Meta Graph API. Merge strategy is Phase 7.
	Metadata Metadata `json:"metadata,omitempty"`
	// SyncVersion increments on successful Meta sync upserts (observability / conflict detection).
	SyncVersion int        `json:"sync_version,omitempty"`
	LastSyncAt  *time.Time `json:"last_sync_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

// WorkspaceID returns the tenant workspace id (alias of TeamID).
func (t *Template) WorkspaceID() uuid.UUID {
	if t == nil {
		return uuid.Nil
	}
	return t.TeamID
}

// CategoryValue returns the typed category when present and recognized.
func (t *Template) CategoryValue() (Category, bool) {
	if t == nil || t.Category == nil {
		return "", false
	}
	return ParseCategory(*t.Category)
}

// ListFilter constrains TemplateRepository.List / TemplateQueryService.Search.
type ListFilter struct {
	ChannelID *uuid.UUID
	Status    *Status
	Search    string
	Language  string // short or locale form; normalized via LanguageResolver
	Category  string // MARKETING | UTILITY | AUTHENTICATION (case-insensitive)
}

// ParseCategory normalizes a category string to a domain Category.
func ParseCategory(raw string) (Category, bool) {
	switch Category(strings.ToUpper(strings.TrimSpace(raw))) {
	case CategoryMarketing:
		return CategoryMarketing, true
	case CategoryUtility:
		return CategoryUtility, true
	case CategoryAuthentication:
		return CategoryAuthentication, true
	default:
		return "", false
	}
}
