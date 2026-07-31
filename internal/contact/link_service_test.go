package contact

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/provider"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

type linkContactRepositoryStub struct {
	Repository
	contacts map[uuid.UUID]*Contact
}

func (s *linkContactRepositoryStub) FindByID(
	_ context.Context,
	teamID, id uuid.UUID,
) (*Contact, error) {
	item, ok := s.contacts[id]
	if !ok || item.TeamID != teamID || item.DeletedAt != nil {
		return nil, repository.ErrNotFound
	}
	copy := *item
	return &copy, nil
}

type linkTeamRepositoryStub struct {
	team.Repository
	member *team.Member
}

func (s *linkTeamRepositoryStub) FindMemberByUser(
	_ context.Context,
	teamID, userID uuid.UUID,
) (*team.Member, error) {
	if s.member.TeamID != teamID || s.member.UserID != userID {
		return nil, team.ErrMemberNotFound
	}
	return s.member, nil
}

type linkRBACRepositoryStub struct {
	rbac.Repository
}

func (s *linkRBACRepositoryStub) LoadRolePermissions(
	context.Context,
) (map[string]map[string]bool, error) {
	return map[string]map[string]bool{
		"owner": {
			rbac.PermContactView:   true,
			rbac.PermContactUpdate: true,
		},
	}, nil
}

type memoryContactLinkRepository struct {
	contacts map[uuid.UUID]*Contact
	pairs    map[[2]uuid.UUID]time.Time
}

func (r *memoryContactLinkRepository) ListByContactIDs(
	_ context.Context,
	teamID uuid.UUID,
	contactIDs []uuid.UUID,
) (map[uuid.UUID][]LinkedContact, error) {
	result := make(map[uuid.UUID][]LinkedContact)
	for _, ownerID := range contactIDs {
		for pair, linkedAt := range r.pairs {
			var linkedID uuid.UUID
			switch ownerID {
			case pair[0]:
				linkedID = pair[1]
			case pair[1]:
				linkedID = pair[0]
			default:
				continue
			}
			linked := r.contacts[linkedID]
			if linked == nil || linked.TeamID != teamID {
				continue
			}
			result[ownerID] = append(result[ownerID], LinkedContact{
				LinkID:       pair[0],
				ID:           linked.ID,
				ContactName:  linked.ContactName,
				CustomerName: linked.CustomerName,
				Phone:        linked.Phone,
				LinkedAt:     linkedAt,
			})
		}
	}
	return result, nil
}

func (r *memoryContactLinkRepository) Link(
	_ context.Context,
	_ uuid.UUID,
	contactID, linkedContactID, _ uuid.UUID,
) error {
	left, right := orderedContactIDs(contactID, linkedContactID)
	pair := [2]uuid.UUID{left, right}
	if _, exists := r.pairs[pair]; !exists {
		r.pairs[pair] = time.Now().UTC()
	}
	return nil
}

func (r *memoryContactLinkRepository) Unlink(
	_ context.Context,
	_ uuid.UUID,
	contactID, linkedContactID uuid.UUID,
) error {
	left, right := orderedContactIDs(contactID, linkedContactID)
	pair := [2]uuid.UUID{left, right}
	if _, exists := r.pairs[pair]; !exists {
		return repository.ErrNotFound
	}
	delete(r.pairs, pair)
	return nil
}

func TestContactLinksAreBidirectionalAndPreserveOwnership(t *testing.T) {
	ctx := context.Background()
	teamID := uuid.New()
	otherTeamID := uuid.New()
	userID := uuid.New()
	contactAID := uuid.New()
	contactBID := uuid.New()
	crossTeamContactID := uuid.New()
	profileAID := uuid.New()
	conversationAID := uuid.New()

	contacts := map[uuid.UUID]*Contact{
		contactAID: {
			ID:          contactAID,
			TeamID:      teamID,
			ContactName: "Contact A",
			Phone:       "+628111111111",
			Providers: []provider.Profile{{
				ID:              profileAID,
				ContactID:       contactAID,
				ConversationIDs: []uuid.UUID{conversationAID},
			}},
		},
		contactBID: {
			ID:          contactBID,
			TeamID:      teamID,
			ContactName: "Contact B",
			Phone:       "+628222222222",
		},
		crossTeamContactID: {
			ID:          crossTeamContactID,
			TeamID:      otherTeamID,
			ContactName: "Other Team",
		},
	}
	contactRepo := &linkContactRepositoryStub{contacts: contacts}
	linkRepo := &memoryContactLinkRepository{
		contacts: contacts,
		pairs:    make(map[[2]uuid.UUID]time.Time),
	}
	teamRepo := &linkTeamRepositoryStub{member: &team.Member{
		ID:     uuid.New(),
		TeamID: teamID,
		UserID: userID,
	}}
	rbacService := rbac.NewService(&linkRBACRepositoryStub{})
	if err := rbacService.WarmCache(ctx); err != nil {
		t.Fatalf("warm RBAC cache: %v", err)
	}
	service := NewService(contactRepo, teamRepo, nil, rbacService)
	service.SetLinkRepository(linkRepo)

	linked, err := service.LinkContact(
		ctx, teamID, userID, "owner", contactAID, contactBID,
	)
	if err != nil {
		t.Fatalf("link contacts: %v", err)
	}
	if linked.ID != contactBID {
		t.Fatalf("linked contact = %s, want %s", linked.ID, contactBID)
	}
	if _, err := service.LinkContact(
		ctx, teamID, userID, "owner", contactBID, contactAID,
	); err != nil {
		t.Fatalf("duplicate reverse link must be idempotent: %v", err)
	}
	if len(linkRepo.pairs) != 1 {
		t.Fatalf("stored pairs = %d, want 1", len(linkRepo.pairs))
	}

	fromA, err := service.ListLinks(ctx, teamID, userID, "owner", contactAID)
	if err != nil || len(fromA) != 1 || fromA[0].ID != contactBID {
		t.Fatalf("links from A = %#v, err = %v", fromA, err)
	}
	fromB, err := service.ListLinks(ctx, teamID, userID, "owner", contactBID)
	if err != nil || len(fromB) != 1 || fromB[0].ID != contactAID {
		t.Fatalf("links from B = %#v, err = %v", fromB, err)
	}
	profile := contacts[contactAID].Providers[0]
	if profile.ContactID != contactAID || profile.ConversationIDs[0] != conversationAID {
		t.Fatal("linking contacts changed provider or conversation ownership")
	}
	if contacts[contactAID].Phone != "+628111111111" || contacts[contactBID].Phone != "+628222222222" {
		t.Fatal("linking contacts must not mutate contact CRM fields")
	}
	if contacts[contactAID].ContactName != "Contact A" || contacts[contactBID].ContactName != "Contact B" {
		t.Fatal("linking contacts must not mutate contact names")
	}

	if _, err := service.LinkContact(
		ctx, teamID, userID, "owner", contactAID, contactAID,
	); err == nil {
		t.Fatal("self-link must fail")
	}
	if _, err := service.LinkContact(
		ctx, teamID, userID, "owner", contactAID, crossTeamContactID,
	); !serviceerrors.IsNotFound(err) {
		t.Fatalf("cross-team link error = %v, want not found", err)
	}

	if err := service.UnlinkContact(
		ctx, teamID, userID, "owner", contactBID, contactAID,
	); err != nil {
		t.Fatalf("unlink contacts: %v", err)
	}
	fromA, _ = service.ListLinks(ctx, teamID, userID, "owner", contactAID)
	fromB, _ = service.ListLinks(ctx, teamID, userID, "owner", contactBID)
	if len(fromA) != 0 || len(fromB) != 0 {
		t.Fatalf("links remain after unlink: A=%#v B=%#v", fromA, fromB)
	}
}
