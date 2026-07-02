package meta

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/pkg/cryptoutil"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

const (
	PaymentStatusConfigured = "configured"
	PaymentStatusMissing    = "missing"
)

type PaymentStatusResult struct {
	Status string `json:"status"`
}

type PaymentLinkResult struct {
	URL string `json:"url"`
}

type PaymentService struct {
	meta     MetaClient
	channels channel.Repository
	cipher   *cryptoutil.TokenCipher
	cfg      config.MetaConfig
}

func NewPaymentService(meta MetaClient, channels channel.Repository, cipher *cryptoutil.TokenCipher, cfg config.MetaConfig) *PaymentService {
	return &PaymentService{meta: meta, channels: channels, cipher: cipher, cfg: cfg}
}

func (s *PaymentService) GetPaymentStatus(ctx context.Context, teamID, channelID uuid.UUID) (*PaymentStatusResult, error) {
	configured, err := s.channelPaymentConfigured(ctx, teamID, channelID)
	if err != nil {
		return nil, err
	}
	if configured {
		return &PaymentStatusResult{Status: PaymentStatusConfigured}, nil
	}
	return &PaymentStatusResult{Status: PaymentStatusMissing}, nil
}

func (s *PaymentService) GetPaymentLink(ctx context.Context, teamID, channelID uuid.UUID) (*PaymentLinkResult, error) {
	ch, err := s.channels.FindByID(ctx, teamID, channelID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if ch.Status != channel.StatusConnected {
		return nil, serviceerrors.NewValidation("channel_id", "channel must be connected before adding payment")
	}

	url := strings.TrimSpace(s.cfg.PaymentSetupURL)
	if url == "" {
		url = defaultPaymentSetupURL(ch)
	} else {
		url = applyPaymentURLPlaceholders(url, ch)
	}
	if strings.TrimSpace(url) == "" {
		return nil, serviceerrors.NewValidation("payment", "payment setup URL is not available for this channel")
	}
	return &PaymentLinkResult{URL: url}, nil
}

func (s *PaymentService) TeamHasPaymentConfigured(ctx context.Context, teamID uuid.UUID) (bool, error) {
	connected := channel.StatusConnected
	page, err := s.channels.List(ctx, teamID, channel.ListFilter{Status: &connected}, repository.PageParams{Page: 1, PageSize: 100})
	if err != nil {
		return false, serviceerrors.MapRepository(err)
	}
	for _, ch := range page.Items {
		ok, err := s.channelPaymentConfigured(ctx, teamID, ch.ID)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func (s *PaymentService) ChannelPaymentConfigured(ctx context.Context, teamID, channelID uuid.UUID) (bool, error) {
	return s.channelPaymentConfigured(ctx, teamID, channelID)
}

func (s *PaymentService) channelPaymentConfigured(ctx context.Context, teamID, channelID uuid.UUID) (bool, error) {
	ch, err := s.channels.FindByID(ctx, teamID, channelID)
	if err != nil {
		return false, serviceerrors.MapRepository(err)
	}
	if ch.WABAID == nil || strings.TrimSpace(*ch.WABAID) == "" {
		return false, nil
	}
	if s.meta == nil || s.cipher == nil {
		return false, nil
	}
	creds, err := s.channels.FindCredentials(ctx, channelID)
	if err != nil {
		return false, nil
	}
	token, err := s.cipher.Decrypt(creds.AccessTokenEnc)
	if err != nil || strings.TrimSpace(token) == "" {
		return false, nil
	}
	waba, err := s.meta.GetWABA(ctx, strings.TrimSpace(*ch.WABAID), token)
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(waba.PrimaryFundingID) != "", nil
}

func defaultPaymentSetupURL(ch *channel.Channel) string {
	if ch.BusinessID != nil && strings.TrimSpace(*ch.BusinessID) != "" {
		return "https://business.facebook.com/billing_hub/payment_methods?business_id=" + strings.TrimSpace(*ch.BusinessID)
	}
	if ch.WABAID != nil && strings.TrimSpace(*ch.WABAID) != "" {
		return "https://business.facebook.com/wa/manage/home/?waba_id=" + strings.TrimSpace(*ch.WABAID)
	}
	return ""
}

func applyPaymentURLPlaceholders(template string, ch *channel.Channel) string {
	out := template
	if ch.BusinessID != nil {
		out = strings.ReplaceAll(out, "{business_id}", strings.TrimSpace(*ch.BusinessID))
	}
	if ch.WABAID != nil {
		out = strings.ReplaceAll(out, "{waba_id}", strings.TrimSpace(*ch.WABAID))
	}
	if ch.PhoneNumberID != nil {
		out = strings.ReplaceAll(out, "{phone_number_id}", strings.TrimSpace(*ch.PhoneNumberID))
	}
	return out
}
