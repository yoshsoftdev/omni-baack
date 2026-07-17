package provider

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type memoryRepository struct {
	profiles   map[uuid.UUID]*Profile
	activities []Activity
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{profiles: make(map[uuid.UUID]*Profile)}
}

func (m *memoryRepository) Create(_ context.Context, profile *Profile) error {
	copy := *profile
	m.profiles[profile.ID] = &copy
	return nil
}

func (m *memoryRepository) FindByID(_ context.Context, teamID, id uuid.UUID) (*Profile, error) {
	item, ok := m.profiles[id]
	if !ok || item.TeamID != teamID {
		return nil, repository.ErrNotFound
	}
	copy := *item
	return &copy, nil
}

func (m *memoryRepository) FindByExternalID(_ context.Context, teamID uuid.UUID, kind Kind, providerUserID string) (*Profile, error) {
	for _, item := range m.profiles {
		if item.TeamID == teamID && item.Provider == kind && item.ProviderUserID == providerUserID {
			copy := *item
			return &copy, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (m *memoryRepository) UpdateObserved(_ context.Context, profile *Profile) error {
	if _, ok := m.profiles[profile.ID]; !ok {
		return repository.ErrNotFound
	}
	copy := *profile
	m.profiles[profile.ID] = &copy
	return nil
}

func (m *memoryRepository) List(_ context.Context, teamID uuid.UUID) ([]Profile, error) {
	items := make([]Profile, 0)
	for _, item := range m.profiles {
		if item.TeamID == teamID {
			items = append(items, *item)
		}
	}
	return items, nil
}

func (m *memoryRepository) ListByContact(_ context.Context, teamID, contactID uuid.UUID) ([]Profile, error) {
	items := make([]Profile, 0)
	for _, item := range m.profiles {
		if item.TeamID == teamID && item.ContactID == contactID {
			items = append(items, *item)
		}
	}
	return items, nil
}

func (m *memoryRepository) ListByContactIDs(ctx context.Context, teamID uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID][]Profile, error) {
	out := make(map[uuid.UUID][]Profile)
	for _, contactID := range contactIDs {
		items, _ := m.ListByContact(ctx, teamID, contactID)
		out[contactID] = items
	}
	return out, nil
}

func (m *memoryRepository) Link(_ context.Context, teamID, profileID, targetContactID, actorID uuid.UUID, _ string) (*LinkResult, error) {
	item, err := m.FindByID(context.Background(), teamID, profileID)
	if err != nil {
		return nil, err
	}
	previous := item.ContactID
	item.ContactID = targetContactID
	m.profiles[profileID] = item
	m.activities = append(m.activities, Activity{Type: ActivityLinked, ActorID: &actorID})
	return &LinkResult{Profile: item, PreviousContactID: previous}, nil
}

func (m *memoryRepository) Unlink(_ context.Context, teamID, profileID, actorID uuid.UUID, _ string) (*UnlinkResult, error) {
	item, err := m.FindByID(context.Background(), teamID, profileID)
	if err != nil {
		return nil, err
	}
	newContactID := uuid.New()
	item.ContactID = newContactID
	m.profiles[profileID] = item
	m.activities = append(m.activities, Activity{Type: ActivityUnlinked, ActorID: &actorID})
	return &UnlinkResult{Profile: item, NewContactID: newContactID}, nil
}

func (m *memoryRepository) SetSourceOfTruth(_ context.Context, teamID, profileID, actorID uuid.UUID) (*Profile, error) {
	item, err := m.FindByID(context.Background(), teamID, profileID)
	if err != nil {
		return nil, err
	}
	item.IsSourceOfTruth = true
	m.profiles[profileID] = item
	m.activities = append(m.activities, Activity{Type: ActivitySourceOfTruthChanged, ActorID: &actorID})
	return item, nil
}

func (m *memoryRepository) ListActivities(_ context.Context, _, _ uuid.UUID, _ int) ([]Activity, error) {
	return m.activities, nil
}

func TestProviderProfileLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepository()
	service := NewService(repo, nil, nil)
	teamID, sourceContactID, targetContactID, actorID := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	created, err := service.Create(ctx, &Profile{
		TeamID:         teamID,
		ContactID:      sourceContactID,
		Provider:       WhatsApp,
		ProviderUserID: "+628123456789",
		Phone:          "+628123456789",
	})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if created.ID == uuid.Nil || created.Status != StatusActive {
		t.Fatalf("profile was not initialized: %#v", created)
	}

	linked, err := service.Link(ctx, teamID, actorID, "owner", created.ID, targetContactID, "")
	if err != nil {
		t.Fatalf("link profile: %v", err)
	}
	if linked.PreviousContactID != sourceContactID || linked.Profile.ContactID != targetContactID {
		t.Fatalf("link changed unexpected ownership: %#v", linked)
	}

	source, err := service.SetSourceOfTruth(ctx, teamID, actorID, "owner", created.ID)
	if err != nil || !source.IsSourceOfTruth {
		t.Fatalf("set source of truth: profile=%#v err=%v", source, err)
	}

	unlinked, err := service.Unlink(ctx, teamID, actorID, "owner", created.ID, "")
	if err != nil {
		t.Fatalf("unlink profile: %v", err)
	}
	if unlinked.NewContactID == targetContactID || unlinked.Profile.ContactID != unlinked.NewContactID {
		t.Fatalf("unlink did not create standalone ownership: %#v", unlinked)
	}
}

func TestSourceOfTruthRejectsNonWhatsApp(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepository()
	service := NewService(repo, nil, nil)
	profile, err := service.Create(ctx, &Profile{
		TeamID: uuid.New(), ContactID: uuid.New(), Provider: Instagram, ProviderUserID: "ig-user",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.SetSourceOfTruth(ctx, profile.TeamID, uuid.New(), "owner", profile.ID)
	if !errors.Is(err, ErrSourceOfTruthMustBeWhatsApp) {
		t.Fatalf("expected WhatsApp source-of-truth error, got %v", err)
	}
}

func TestCanonicalProviderMigrationAndCompatibilityContracts(t *testing.T) {
	migration, err := os.ReadFile("../../migrations/000037_canonical_provider.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(migration)
	for _, contract := range []string{
		"CREATE TABLE provider_profiles",
		"ALTER TABLE conversations",
		"provider_profile_id UUID",
		"conversation provider profile is immutable",
		"ALTER COLUMN provider_profile_id SET NOT NULL",
		"INSERT INTO provider_profiles",
		"provider = 'whatsapp'",
		"contact_provider_activities",
	} {
		if !strings.Contains(sql, contract) {
			t.Errorf("migration missing contract %q", contract)
		}
	}

	conversationRepository, err := os.ReadFile("../conversation/repository.go")
	if err != nil {
		t.Fatal(err)
	}
	repositorySource := string(conversationRepository)
	if !strings.Contains(repositorySource, "COALESCE(pp.contact_id, c.contact_id)") {
		t.Error("inbox does not resolve current Contact through Provider Profile ownership")
	}
	if !strings.Contains(repositorySource, "contact_id, provider_profile_id") {
		t.Error("legacy contact_id is not retained alongside canonical ownership")
	}

	contactRepository, err := os.ReadFile("../contact/repository.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contactRepository), "p.username ILIKE") {
		t.Error("contact search does not include provider usernames")
	}
}

var _ Repository = (*memoryRepository)(nil)
