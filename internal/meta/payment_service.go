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
	PaymentStatusRestricted = "restricted"

	PaymentMessageMissing = "Metode pembayaran belum dikonfigurasi."
	PaymentMessageOK      = "Payment OK"
	PaymentMessageRestricted = "Akun WhatsApp Business sedang dibatasi oleh Meta. Silakan buka WhatsApp Manager untuk melihat dan menyelesaikan masalah."
)

type PaymentStatusResult struct {
	Status             string `json:"status"`
	Message            string `json:"message"`
	WABAStatus         string `json:"waba_status,omitempty"`
	HasPrimaryFunding  bool   `json:"has_primary_funding"`
	PrimaryFundingID   string `json:"primary_funding_id,omitempty"`
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
	return s.inspectChannelPayment(ctx, teamID, channelID)
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
		ok, err := s.ChannelPaymentConfigured(ctx, teamID, ch.ID)
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
	result, err := s.inspectChannelPayment(ctx, teamID, channelID)
	if err != nil {
		return false, err
	}
	return result.Status == PaymentStatusConfigured, nil
}

func (s *PaymentService) channelPaymentConfigured(ctx context.Context, teamID, channelID uuid.UUID) (bool, error) {
	return s.ChannelPaymentConfigured(ctx, teamID, channelID)
}

func (s *PaymentService) inspectChannelPayment(ctx context.Context, teamID, channelID uuid.UUID) (*PaymentStatusResult, error) {
	ch, err := s.channels.FindByID(ctx, teamID, channelID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if ch.WABAID == nil || strings.TrimSpace(*ch.WABAID) == "" {
		return classifyPaymentStatus("", ""), nil
	}
	if s.meta == nil || s.cipher == nil {
		return classifyPaymentStatus("", ""), nil
	}
	creds, err := s.channels.FindCredentials(ctx, channelID)
	if err != nil {
		return classifyPaymentStatus("", ""), nil
	}
	token, err := s.cipher.Decrypt(creds.AccessTokenEnc)
	if err != nil || strings.TrimSpace(token) == "" {
		return classifyPaymentStatus("", ""), nil
	}

	waba, err := s.meta.GetWABA(ctx, strings.TrimSpace(*ch.WABAID), token)
	if err != nil || waba == nil {
		return classifyPaymentStatus("", ""), nil
	}
	return classifyPaymentStatus(waba.PrimaryFundingID, waba.Status), nil
}

// classifyPaymentStatus maps Meta WABA fields to payment UI state.
// GET /{WABA_ID}?fields=primary_funding_id,status
func classifyPaymentStatus(primaryFundingID, wabaStatus string) *PaymentStatusResult {
	fundingID := strings.TrimSpace(primaryFundingID)
	status := strings.TrimSpace(wabaStatus)
	hasFunding := fundingID != ""

	result := &PaymentStatusResult{
		WABAStatus:        status,
		HasPrimaryFunding: hasFunding,
	}
	if hasFunding {
		result.PrimaryFundingID = fundingID
	}

	if isWABARestricted(status) {
		result.Status = PaymentStatusRestricted
		result.Message = PaymentMessageRestricted
		return result
	}
	if !hasFunding {
		result.Status = PaymentStatusMissing
		result.Message = PaymentMessageMissing
		return result
	}
	result.Status = PaymentStatusConfigured
	result.Message = PaymentMessageOK
	return result
}

func isWABARestricted(status string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(status))
	return normalized == "RESTRICTED"
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
