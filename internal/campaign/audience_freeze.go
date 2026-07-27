package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign/recipient"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// AudienceContactLookup loads contacts for CRM field values and opt-out at snapshot time.
// Phone / WA identity must come from WhatsAppRecipientResolver via RecipientSnapshotBuilder.
type AudienceContactLookup interface {
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*contact.Contact, error)
	ListByIDs(ctx context.Context, teamID uuid.UUID, ids []uuid.UUID) ([]contact.Contact, error)
}

// AudienceTagLookup expands tag audiences into contact IDs.
type AudienceTagLookup interface {
	ListContactIDsByTag(ctx context.Context, teamID, tagID uuid.UUID) ([]uuid.UUID, error)
}

type recipientSnapshotDeps struct {
	builder  *recipient.RecipientSnapshotBuilder
	contacts AudienceContactLookup
	tags     AudienceTagLookup
}

// WithRecipientSnapshotting freezes campaign_recipients at create/schedule time.
func WithRecipientSnapshotting(
	builder *recipient.RecipientSnapshotBuilder,
	contacts AudienceContactLookup,
	tags AudienceTagLookup,
) ServiceOption {
	return func(s *Service) {
		s.snapshots = &recipientSnapshotDeps{
			builder:  builder,
			contacts: contacts,
			tags:     tags,
		}
	}
}

func (s *Service) buildRecipientSnapshots(
	ctx context.Context,
	teamID uuid.UUID,
	campaignID uuid.UUID,
	jobID uuid.UUID,
	audiences []Audience,
	variableMapping json.RawMessage,
	sampleValues json.RawMessage,
	now time.Time,
) ([]Recipient, int, int, int, error) {
	if s.snapshots == nil || s.snapshots.builder == nil {
		return nil, 0, 0, 0, serviceerrors.NewValidation("audiences", "recipient snapshotting is not configured")
	}

	contacts, err := s.collectAudienceContacts(ctx, teamID, audiences)
	if err != nil {
		return nil, 0, 0, 0, err
	}

	inputs := make([]recipient.ContactSnapshotInput, 0, len(contacts))
	for _, c := range contacts {
		inputs = append(inputs, recipient.ContactSnapshotInput{
			ContactID:   c.ID,
			OptedOut:    c.MarketingStatus == contact.MarketingUnsubscribed,
			FieldValues: contactFieldValues(c),
		})
	}

	mapping, err := ParseVariableMapping(variableMapping)
	if err != nil {
		return nil, 0, 0, 0, serviceerrors.NewValidation("variable_mapping", err.Error())
	}
	samples := ParseSampleValues(sampleValues)

	buildResult, err := s.snapshots.builder.BuildMany(
		ctx,
		teamID,
		inputs,
		toVariableMappingSources(mapping),
		samples,
	)
	if err != nil {
		if errors.Is(err, recipient.ErrNoEligibleRecipients) {
			return nil, 0, 0, 0, serviceerrors.NewValidation(
				"audiences",
				"no eligible WhatsApp recipients; every contact must have an active WhatsApp provider",
			)
		}
		return nil, 0, 0, 0, err
	}

	jobRef := jobID
	out := make([]Recipient, 0, len(buildResult.Snapshots))
	for _, snap := range buildResult.Snapshots {
		vars, err := snap.VariablesJSON()
		if err != nil {
			return nil, 0, 0, 0, err
		}
		rec := Recipient{
			ID:                uuid.New(),
			CampaignID:        campaignID,
			TeamID:            teamID,
			ContactID:         snap.ContactID,
			JobID:             &jobRef,
			ProviderProfileID: snap.ProviderProfileID,
			PhoneNumber:       snap.PhoneNumber,
			WAID:              snap.WAID,
			Variables:         vars,
			Status:            RecipientStatus(snap.Status),
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if msg := snap.SkipReason.Message(); msg != "" {
			rec.LastError = &msg
		}
		out = append(out, rec)
	}

	return out, buildResult.EligibleCount, buildResult.SkippedOptOut, buildResult.SkippedNoProvider + buildResult.SkippedInvalid, nil
}

func (s *Service) collectAudienceContacts(ctx context.Context, teamID uuid.UUID, audiences []Audience) ([]contact.Contact, error) {
	seen := map[uuid.UUID]struct{}{}
	var contacts []contact.Contact

	for _, aud := range audiences {
		switch aud.SourceType {
		case AudienceContact:
			c, err := s.snapshots.contacts.FindByID(ctx, teamID, aud.SourceID)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					continue
				}
				return nil, err
			}
			if _, ok := seen[c.ID]; !ok {
				seen[c.ID] = struct{}{}
				contacts = append(contacts, *c)
			}
		case AudienceTag:
			ids, err := s.snapshots.tags.ListContactIDsByTag(ctx, teamID, aud.SourceID)
			if err != nil {
				return nil, err
			}
			batch, err := s.snapshots.contacts.ListByIDs(ctx, teamID, ids)
			if err != nil {
				return nil, err
			}
			for _, c := range batch {
				if _, ok := seen[c.ID]; !ok {
					seen[c.ID] = struct{}{}
					contacts = append(contacts, c)
				}
			}
		default:
			// Segment audiences are rejected at validation; ignore if present.
			continue
		}
	}
	return contacts, nil
}

func contactFieldValues(c contact.Contact) map[string]string {
	fields := map[string]string{
		"name":          strings.TrimSpace(c.CustomerName),
		"customer_name": strings.TrimSpace(c.CustomerName),
		"contact_name":  strings.TrimSpace(c.ContactName),
	}
	if c.Email != nil {
		fields["email"] = strings.TrimSpace(*c.Email)
	}
	if c.Notes != nil {
		fields["notes"] = strings.TrimSpace(*c.Notes)
	}
	return fields
}

func toVariableMappingSources(m VariableMapping) recipient.VariableMappingSources {
	out := recipient.VariableMappingSources{
		Body:      m.Body,
		ButtonURL: map[string]map[string]string{},
	}
	if m.Header != nil {
		out.HeaderText = m.Header.Text
	}
	for index, btn := range m.Buttons {
		if len(btn.URL) == 0 {
			continue
		}
		out.ButtonURL[index] = btn.URL
	}
	return out
}
