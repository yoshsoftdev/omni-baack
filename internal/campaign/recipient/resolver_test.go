package recipient

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/provider"
)

type memoryProviders struct {
	byContact map[uuid.UUID][]provider.Profile
	err       error
}

func (m *memoryProviders) ListByContact(_ context.Context, _, contactID uuid.UUID) ([]provider.Profile, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.byContact[contactID], nil
}

func (m *memoryProviders) ListByContactIDs(_ context.Context, _ uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID][]provider.Profile, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make(map[uuid.UUID][]provider.Profile, len(contactIDs))
	for _, id := range contactIDs {
		if profiles, ok := m.byContact[id]; ok {
			out[id] = profiles
		}
	}
	return out, nil
}

func TestResolvePrefersSourceOfTruthWhatsApp(t *testing.T) {
	contactID := uuid.New()
	sot := uuid.New()
	other := uuid.New()
	dir := &memoryProviders{byContact: map[uuid.UUID][]provider.Profile{
		contactID: {
			{
				ID:              other,
				ContactID:       contactID,
				Provider:        provider.WhatsApp,
				ProviderUserID:  "628111111111",
				Phone:           "628111111111",
				Status:          provider.StatusActive,
				IsSourceOfTruth: false,
			},
			{
				ID:              sot,
				ContactID:       contactID,
				Provider:        provider.WhatsApp,
				ProviderUserID:  "628222222222",
				Phone:           "628222222222",
				Status:          provider.StatusActive,
				IsSourceOfTruth: true,
			},
		},
	}}

	got, err := NewWhatsAppRecipientResolver(dir).Resolve(context.Background(), uuid.New(), contactID)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ProviderProfileID != sot {
		t.Fatalf("expected SoT profile %s, got %s", sot, got.ProviderProfileID)
	}
	if got.PhoneNumber != "628222222222" || got.WAID != "628222222222" {
		t.Fatalf("unexpected recipient %#v", got)
	}
}

func TestResolveIgnoresNonWhatsAppAndInactive(t *testing.T) {
	contactID := uuid.New()
	wa := uuid.New()
	dir := &memoryProviders{byContact: map[uuid.UUID][]provider.Profile{
		contactID: {
			{
				ID:             uuid.New(),
				ContactID:      contactID,
				Provider:       provider.Instagram,
				ProviderUserID: "ig:abc",
				Status:         provider.StatusActive,
			},
			{
				ID:             uuid.New(),
				ContactID:      contactID,
				Provider:       provider.WhatsApp,
				ProviderUserID: "628999999999",
				Phone:          "628999999999",
				Status:         provider.StatusUnlinked,
			},
			{
				ID:             wa,
				ContactID:      contactID,
				Provider:       provider.WhatsApp,
				ProviderUserID: "628333333333",
				Phone:          "628333333333",
				Status:         provider.StatusActive,
			},
		},
	}}

	got, err := NewWhatsAppRecipientResolver(dir).Resolve(context.Background(), uuid.New(), contactID)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ProviderProfileID != wa {
		t.Fatalf("expected active WA profile %s, got %s", wa, got.ProviderProfileID)
	}
}

func TestResolveFallsBackToProviderUserID(t *testing.T) {
	contactID := uuid.New()
	profileID := uuid.New()
	dir := &memoryProviders{byContact: map[uuid.UUID][]provider.Profile{
		contactID: {{
			ID:             profileID,
			ContactID:      contactID,
			Provider:       provider.WhatsApp,
			ProviderUserID: "628444444444",
			Status:         provider.StatusActive,
		}},
	}}

	got, err := NewWhatsAppRecipientResolver(dir).Resolve(context.Background(), uuid.New(), contactID)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.PhoneNumber != "628444444444" || got.WAID != "628444444444" {
		t.Fatalf("unexpected recipient %#v", got)
	}
}

func TestResolveNoWhatsAppProvider(t *testing.T) {
	contactID := uuid.New()
	dir := &memoryProviders{byContact: map[uuid.UUID][]provider.Profile{
		contactID: {{
			ID:             uuid.New(),
			ContactID:      contactID,
			Provider:       provider.Messenger,
			ProviderUserID: "ms:1",
			Status:         provider.StatusActive,
		}},
	}}

	_, err := NewWhatsAppRecipientResolver(dir).Resolve(context.Background(), uuid.New(), contactID)
	if !errors.Is(err, ErrNoWhatsAppProvider) {
		t.Fatalf("expected ErrNoWhatsAppProvider, got %v", err)
	}
}

func TestResolveManyOmitsMissingProviders(t *testing.T) {
	withWA := uuid.New()
	withoutWA := uuid.New()
	dir := &memoryProviders{byContact: map[uuid.UUID][]provider.Profile{
		withWA: {{
			ID:             uuid.New(),
			ContactID:      withWA,
			Provider:       provider.WhatsApp,
			ProviderUserID: "628555555555",
			Phone:          "628555555555",
			Status:         provider.StatusActive,
		}},
		withoutWA: {{
			ID:             uuid.New(),
			ContactID:      withoutWA,
			Provider:       provider.Instagram,
			ProviderUserID: "ig:x",
			Status:         provider.StatusActive,
		}},
	}}

	got, err := NewWhatsAppRecipientResolver(dir).ResolveMany(context.Background(), uuid.New(), []uuid.UUID{withWA, withoutWA})
	if err != nil {
		t.Fatalf("ResolveMany: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 resolved, got %d", len(got))
	}
	if _, ok := got[withWA]; !ok {
		t.Fatal("expected withWA in result")
	}
	if _, ok := got[withoutWA]; ok {
		t.Fatal("withoutWA must be omitted")
	}
}
