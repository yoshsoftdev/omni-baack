package recipient

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ContactSnapshotInput carries CRM fields needed to build a recipient snapshot.
// Phone / WA identity must come from WhatsAppRecipientResolver — not from these fields.
type ContactSnapshotInput struct {
	ContactID uuid.UUID
	// OptedOut marks marketing unsubscribed contacts (skipped, clear reason).
	OptedOut bool
	// FieldValues maps variable sources (name, email, …) to resolved CRM values.
	// Do not put legacy Contact.Phone here; "phone" is filled from the WhatsApp recipient.
	FieldValues map[string]string
}

// VariableMappingSources describes template variable sources keyed by component path.
// Body example: {"1": "name", "2": "phone"}.
// Values are either contact field keys or literal URLs.
type VariableMappingSources struct {
	HeaderText map[string]string
	Body       map[string]string
	// ButtonURL[buttonIndex][varIndex] = source
	ButtonURL map[string]map[string]string
}

// Resolver is the WhatsApp identity port used by RecipientSnapshotBuilder.
type Resolver interface {
	Resolve(ctx context.Context, teamID, contactID uuid.UUID) (*WhatsAppRecipient, error)
	ResolveMany(ctx context.Context, teamID uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID]*WhatsAppRecipient, error)
}

// RecipientSnapshotBuilder builds frozen campaign recipient snapshots.
// It uses WhatsAppRecipientResolver for identity and never touches Canonical Provider itself.
type RecipientSnapshotBuilder struct {
	resolver Resolver
}

// NewRecipientSnapshotBuilder constructs a snapshot builder.
func NewRecipientSnapshotBuilder(resolver Resolver) *RecipientSnapshotBuilder {
	return &RecipientSnapshotBuilder{resolver: resolver}
}

// BuildResult is the outcome of building snapshots for an audience.
type BuildResult struct {
	Snapshots          []Snapshot
	EligibleCount      int
	SkippedOptOut      int
	SkippedNoProvider  int
	SkippedInvalid     int
}

// BuildMany resolves WhatsApp identity and freezes variables for each contact.
// Contacts without WhatsApp are included as skipped snapshots with a clear reason.
// Returns ErrNoEligibleRecipients when EligibleCount == 0.
func (b *RecipientSnapshotBuilder) BuildMany(
	ctx context.Context,
	teamID uuid.UUID,
	contacts []ContactSnapshotInput,
	mapping VariableMappingSources,
	sampleValues map[string]string,
) (*BuildResult, error) {
	if b == nil || b.resolver == nil {
		return nil, fmt.Errorf("recipient snapshot builder: resolver not configured")
	}

	ids := make([]uuid.UUID, 0, len(contacts))
	for _, c := range contacts {
		ids = append(ids, c.ContactID)
	}
	resolved, err := b.resolver.ResolveMany(ctx, teamID, ids)
	if err != nil {
		return nil, err
	}

	result := &BuildResult{
		Snapshots: make([]Snapshot, 0, len(contacts)),
	}

	for _, c := range contacts {
		snap := b.buildOne(c, resolved[c.ContactID], mapping, sampleValues)
		result.Snapshots = append(result.Snapshots, snap)
		switch snap.SkipReason {
		case SkipReasonOptOut:
			result.SkippedOptOut++
		case SkipReasonNoWhatsAppProvider:
			result.SkippedNoProvider++
		case SkipReasonInvalidAddress:
			result.SkippedInvalid++
		default:
			if snap.Status == SnapshotPending {
				result.EligibleCount++
			}
		}
	}

	if result.EligibleCount == 0 {
		return result, ErrNoEligibleRecipients
	}
	return result, nil
}

func (b *RecipientSnapshotBuilder) buildOne(
	c ContactSnapshotInput,
	wa *WhatsAppRecipient,
	mapping VariableMappingSources,
	sampleValues map[string]string,
) Snapshot {
	snap := Snapshot{
		ContactID: c.ContactID,
		Variables: map[string]string{},
		Status:    SnapshotSkipped,
	}

	if c.OptedOut {
		snap.SkipReason = SkipReasonOptOut
		return snap
	}
	if wa == nil {
		snap.SkipReason = SkipReasonNoWhatsAppProvider
		return snap
	}

	profileID := wa.ProviderProfileID
	snap.ProviderProfileID = &profileID
	snap.PhoneNumber = wa.PhoneNumber
	snap.WAID = wa.WAID
	snap.Variables = resolveVariables(mapping, c.FieldValues, wa, sampleValues)
	snap.Status = SnapshotPending
	snap.SkipReason = SkipReasonNone
	return snap
}

func resolveVariables(
	mapping VariableMappingSources,
	fields map[string]string,
	wa *WhatsAppRecipient,
	sampleValues map[string]string,
) map[string]string {
	out := make(map[string]string)
	apply := func(key, source string) {
		source = strings.TrimSpace(source)
		if source == "" {
			return
		}
		if value := resolveSource(source, fields, wa, sampleValues, key); value != "" {
			out[key] = value
		}
	}

	for index, source := range mapping.HeaderText {
		apply("header."+index, source)
	}
	for index, source := range mapping.Body {
		apply("body."+index, source)
	}
	for btnIndex, vars := range mapping.ButtonURL {
		for varIndex, source := range vars {
			apply("button."+btnIndex+"."+varIndex, source)
		}
	}
	return out
}

func resolveSource(
	source string,
	fields map[string]string,
	wa *WhatsAppRecipient,
	sampleValues map[string]string,
	indexKey string,
) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}

	switch source {
	case "phone":
		if wa != nil && strings.TrimSpace(wa.PhoneNumber) != "" {
			return strings.TrimSpace(wa.PhoneNumber)
		}
	default:
		if fields != nil {
			if v := strings.TrimSpace(fields[source]); v != "" {
				return v
			}
		}
	}

	if sampleValues != nil {
		if v := strings.TrimSpace(sampleValues[indexKey]); v != "" {
			return v
		}
		if v := strings.TrimSpace(sampleValues[source]); v != "" {
			return v
		}
	}

	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return source
	}
	return ""
}
