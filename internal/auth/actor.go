package auth

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// Actor is the authenticated workspace principal for a single HTTP request.
type Actor struct {
	UserID      uuid.UUID
	TeamID      uuid.UUID
	Role        string
	RoleID      uuid.UUID
	Permissions Permissions
	Email       string
	MemberID    uuid.UUID
}

// Permissions is a snapshot of granted permission codes for the actor's role.
type Permissions map[string]bool

func (p Permissions) Has(code string) bool {
	if p == nil {
		return false
	}
	return p[code]
}

// AssertTeam ensures the actor belongs to the requested workspace.
func (a Actor) AssertTeam(teamID uuid.UUID) error {
	if a.TeamID != teamID {
		return serviceerrors.ErrForbidden
	}
	return nil
}

// Validate checks actor invariants after resolution.
func (a Actor) Validate() error {
	if a.UserID == uuid.Nil || a.TeamID == uuid.Nil {
		return fmt.Errorf("invalid actor: missing user or team")
	}
	if a.Permissions == nil {
		return fmt.Errorf("invalid actor: permissions snapshot required")
	}
	return nil
}
