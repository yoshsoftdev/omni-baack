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
	ID              uuid.UUID
	TeamID          uuid.UUID
	Name            string
	Description     string
	Color           string
	ContactCount    int
	IsDynamic       bool
	LastEvaluatedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

type Rule struct {
	ID        uuid.UUID
	SegmentID uuid.UUID
	Field     RuleField
	Operator  RuleOperator
	Value     string
	SortOrder int
	CreatedAt time.Time
}

type ListFilter struct {
	Search string
}
