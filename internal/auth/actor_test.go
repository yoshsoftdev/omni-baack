package auth

import (
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

func TestActor_RequirePermission(t *testing.T) {
	actor := Actor{
		UserID: uuid.New(),
		TeamID: uuid.New(),
		Role:   "owner",
		Permissions: Permissions{
			"campaign.create": true,
		},
	}
	if err := actor.RequirePermission("campaign.create"); err != nil {
		t.Fatalf("expected permission granted: %v", err)
	}
	if err := actor.RequirePermission("campaign.send"); !serviceerrors.IsForbidden(err) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestActor_AssertTeam(t *testing.T) {
	teamID := uuid.New()
	actor := Actor{TeamID: teamID, UserID: uuid.New(), Permissions: Permissions{}}
	if err := actor.AssertTeam(teamID); err != nil {
		t.Fatalf("AssertTeam() = %v", err)
	}
	if err := actor.AssertTeam(uuid.New()); !serviceerrors.IsForbidden(err) {
		t.Fatalf("expected forbidden for mismatched team")
	}
}
