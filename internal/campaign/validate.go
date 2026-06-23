package campaign

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/segment"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

func validateChannelForCampaign(ctx context.Context, channels channel.Repository, teamID, channelID uuid.UUID) error {
	ch, err := channels.FindByID(ctx, teamID, channelID)
	if err != nil {
		return serviceerrors.MapRepository(err)
	}
	if ch.Status != channel.StatusConnected {
		return serviceerrors.NewValidation("channel_id", "channel must be connected")
	}
	ob, err := channels.FindOnboarding(ctx, channelID)
	if err != nil {
		return serviceerrors.MapRepository(err)
	}
	if ob.Status != channel.OnboardingCompleted {
		return serviceerrors.NewValidation("channel_id", "channel onboarding must be completed")
	}
	return nil
}

func validateTemplateForCampaign(ctx context.Context, templates template.Repository, teamID, channelID, templateID uuid.UUID) error {
	tmpl, err := templates.FindByID(ctx, teamID, templateID)
	if err != nil {
		return serviceerrors.MapRepository(err)
	}
	if tmpl.ChannelID != channelID {
		return serviceerrors.NewValidation("template_id", "template does not belong to the selected channel")
	}
	if tmpl.Status != template.StatusApproved {
		return serviceerrors.NewValidation("template_id", "template must be approved by Meta")
	}
	if strings.TrimSpace(tmpl.Name) == "" {
		return serviceerrors.NewValidation("template_id", "template name is required")
	}
	return nil
}

func validateSegmentExists(ctx context.Context, segments segment.Repository, teamID, segmentID uuid.UUID) error {
	_, _, err := segments.FindByID(ctx, teamID, segmentID)
	return serviceerrors.MapRepository(err)
}
