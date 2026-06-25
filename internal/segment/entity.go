package segment

import (
	"time"

	"github.com/google/uuid"
)

type RuleField string

const (
	RuleFieldTag             RuleField = "tag"
	RuleFieldCity            RuleField = "city"
	RuleFieldLastActivity    RuleField = "last_activity"
	RuleFieldMarketingStatus RuleField = "marketing_status"
	RuleFieldSource          RuleField = "source"
)

type RuleOperator string

const (
	OpEquals      RuleOperator = "equals"
	OpNotEquals   RuleOperator = "not_equals"
	OpContains    RuleOperator = "contains"
	OpLessThan    RuleOperator = "less_than"
	OpGreaterThan RuleOperator = "greater_than"
)

type Segment struct {
	ID              uuid.UUID  `json:"id"`
	TeamID          uuid.UUID  `json:"team_id"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	Color           string     `json:"color"`
	ContactCount    int        `json:"contact_count"`
	IsDynamic       bool       `json:"is_dynamic"`
	LastEvaluatedAt *time.Time `json:"last_evaluated_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
}

type Rule struct {
	ID        uuid.UUID    `json:"id"`
	SegmentID uuid.UUID    `json:"segment_id"`
	Field     RuleField    `json:"field"`
	Operator  RuleOperator `json:"operator"`
	Value     string       `json:"value"`
	SortOrder int          `json:"sort_order"`
	CreatedAt time.Time    `json:"created_at"`
}

type ListFilter struct {
	Search string
}
