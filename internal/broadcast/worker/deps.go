package worker

import (
	"context"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/template"
)

// CampaignStore loads campaigns and updates recipient delivery state.
type CampaignStore interface {
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*campaign.Campaign, error)
	UpdateRecipientDelivery(ctx context.Context, id uuid.UUID, from, to campaign.RecipientStatus, update campaign.RecipientDeliveryUpdate) error
}

// TemplateStore loads synced Meta templates.
type TemplateStore interface {
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*template.Template, error)
}

// ChannelStore loads channels and credentials for Meta sends.
type ChannelStore interface {
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*channel.Channel, error)
	FindCredentials(ctx context.Context, channelID uuid.UUID) (*channel.Credentials, error)
}

// CampaignCancelChecker reports whether a campaign cancel flag is set.
type CampaignCancelChecker interface {
	IsSet(ctx context.Context, campaignID uuid.UUID) (bool, error)
}

// SendFinalizer atomically commits send outcomes with counters.
type SendFinalizer interface {
	FinalizeRecipientSent(ctx context.Context, in campaign.FinalizeSentInput) (bool, error)
	FinalizeRecipientFailed(ctx context.Context, in campaign.FinalizeFailedInput) (bool, error)
}
