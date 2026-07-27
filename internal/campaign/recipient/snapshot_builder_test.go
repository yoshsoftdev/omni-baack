package recipient

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type stubResolver struct {
	byID map[uuid.UUID]*WhatsAppRecipient
	err  error
}

func (s *stubResolver) Resolve(_ context.Context, _, contactID uuid.UUID) (*WhatsAppRecipient, error) {
	if s.err != nil {
		return nil, s.err
	}
	rec, ok := s.byID[contactID]
	if !ok {
		return nil, ErrNoWhatsAppProvider
	}
	return rec, nil
}

func (s *stubResolver) ResolveMany(_ context.Context, _ uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID]*WhatsAppRecipient, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make(map[uuid.UUID]*WhatsAppRecipient)
	for _, id := range contactIDs {
		if rec, ok := s.byID[id]; ok {
			out[id] = rec
		}
	}
	return out, nil
}

func TestSnapshotBuilderSkipsMissingProviderAndContinues(t *testing.T) {
	eligible := uuid.New()
	missing := uuid.New()
	profileID := uuid.New()
	resolver := &stubResolver{byID: map[uuid.UUID]*WhatsAppRecipient{
		eligible: {
			ContactID:         eligible,
			ProviderProfileID: profileID,
			PhoneNumber:       "628123456789",
			WAID:              "628123456789",
		},
	}}

	result, err := NewRecipientSnapshotBuilder(resolver).BuildMany(
		context.Background(),
		uuid.New(),
		[]ContactSnapshotInput{
			{ContactID: eligible, FieldValues: map[string]string{"name": "Ada"}},
			{ContactID: missing, FieldValues: map[string]string{"name": "Bob"}},
		},
		VariableMappingSources{Body: map[string]string{"1": "name", "2": "phone"}},
		nil,
	)
	if err != nil {
		t.Fatalf("BuildMany: %v", err)
	}
	if result.EligibleCount != 1 || result.SkippedNoProvider != 1 {
		t.Fatalf("counts: eligible=%d skippedNoProvider=%d", result.EligibleCount, result.SkippedNoProvider)
	}
	if len(result.Snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(result.Snapshots))
	}

	pending := result.Snapshots[0]
	if pending.Status != SnapshotPending || pending.PhoneNumber != "628123456789" {
		t.Fatalf("pending snapshot %#v", pending)
	}
	if pending.Variables["body.1"] != "Ada" || pending.Variables["body.2"] != "628123456789" {
		t.Fatalf("variables %#v", pending.Variables)
	}
	if pending.ProviderProfileID == nil || *pending.ProviderProfileID != profileID {
		t.Fatalf("provider_profile_id %#v", pending.ProviderProfileID)
	}

	skipped := result.Snapshots[1]
	if skipped.Status != SnapshotSkipped || skipped.SkipReason != SkipReasonNoWhatsAppProvider {
		t.Fatalf("skipped snapshot %#v", skipped)
	}
}

func TestSnapshotBuilderOptOut(t *testing.T) {
	id := uuid.New()
	resolver := &stubResolver{byID: map[uuid.UUID]*WhatsAppRecipient{
		id: {ContactID: id, ProviderProfileID: uuid.New(), PhoneNumber: "628111111111", WAID: "628111111111"},
	}}

	result, err := NewRecipientSnapshotBuilder(resolver).BuildMany(
		context.Background(),
		uuid.New(),
		[]ContactSnapshotInput{{ContactID: id, OptedOut: true}},
		VariableMappingSources{},
		nil,
	)
	if !errors.Is(err, ErrNoEligibleRecipients) {
		t.Fatalf("expected ErrNoEligibleRecipients, got %v", err)
	}
	if result == nil || result.SkippedOptOut != 1 || result.EligibleCount != 0 {
		t.Fatalf("unexpected result %#v", result)
	}
	if result.Snapshots[0].SkipReason != SkipReasonOptOut {
		t.Fatalf("skip reason %q", result.Snapshots[0].SkipReason)
	}
}

func TestSnapshotBuilderFailsWhenZeroEligible(t *testing.T) {
	id := uuid.New()
	resolver := &stubResolver{byID: map[uuid.UUID]*WhatsAppRecipient{}}

	result, err := NewRecipientSnapshotBuilder(resolver).BuildMany(
		context.Background(),
		uuid.New(),
		[]ContactSnapshotInput{{ContactID: id}},
		VariableMappingSources{},
		nil,
	)
	if !errors.Is(err, ErrNoEligibleRecipients) {
		t.Fatalf("expected ErrNoEligibleRecipients, got %v", err)
	}
	if result.EligibleCount != 0 || result.SkippedNoProvider != 1 {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestSnapshotBuilderDoesNotTouchProviderDirectory(t *testing.T) {
	// Compile-time / construction check: builder only needs Resolver, not ProviderDirectory.
	var _ Resolver = (*WhatsAppRecipientResolver)(nil)
	id := uuid.New()
	builder := NewRecipientSnapshotBuilder(&stubResolver{byID: map[uuid.UUID]*WhatsAppRecipient{
		id: {ContactID: id, ProviderProfileID: uuid.New(), PhoneNumber: "628777777777", WAID: "628777777777"},
	}})
	if builder.resolver == nil {
		t.Fatal("resolver required")
	}
}
