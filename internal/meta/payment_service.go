package meta

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/pkg/cryptoutil"
	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

const (
	PaymentStatusConfigured  = "configured"
	PaymentStatusMissing     = "missing"
	PaymentStatusRestricted  = "restricted"
	PaymentStatusUnavailable = "unavailable"

	PaymentMessageMissing    = "Metode pembayaran belum dikonfigurasi."
	PaymentMessageOK         = "Payment OK"
	PaymentMessageRestricted = "Akun WhatsApp Business sedang dibatasi oleh Meta. Silakan buka WhatsApp Manager untuk melihat dan menyelesaikan masalah."
	PaymentMessageUnavailable = "Tidak dapat memastikan status pembayaran dari Meta saat ini. Coba refresh atau buka Meta Billing."
	PaymentCheckSourceFunding = "primary_funding_id"
	PaymentCheckSourceHealth  = "health_status"
)

type PaymentStatusResult struct {
	Status                     string `json:"status"`
	Message                    string `json:"message"`
	WABAStatus                 string `json:"waba_status,omitempty"`
	BusinessVerificationStatus string `json:"business_verification_status,omitempty"`
	HasPrimaryFunding          bool   `json:"has_primary_funding"`
	PrimaryFundingID           string `json:"primary_funding_id,omitempty"`
	FundingCheckError          string `json:"funding_check_error,omitempty"`
	CheckSource                string `json:"check_source,omitempty"`
	HealthCanSendMessage       string `json:"health_can_send_message,omitempty"`
	HealthPaymentErrorCode     int    `json:"health_payment_error_code,omitempty"`
	HealthPaymentError         string `json:"health_payment_error,omitempty"`
	// Debug / audit fields — which Meta IDs were used for the check.
	RequestedWABAID  string `json:"requested_waba_id,omitempty"`
	StoredBusinessID string `json:"stored_business_id,omitempty"`
	MetaObjectID     string `json:"meta_object_id,omitempty"`
	OwnerBusinessID  string `json:"owner_business_id,omitempty"`
}

type PaymentLinkResult struct {
	URL string `json:"url"`
}

type PaymentService struct {
	meta     MetaClient
	channels channel.Repository
	cipher   *cryptoutil.TokenCipher
	cfg      config.MetaConfig
	log      *slog.Logger
}

func NewPaymentService(meta MetaClient, channels channel.Repository, cipher *cryptoutil.TokenCipher, cfg config.MetaConfig, log *slog.Logger) *PaymentService {
	if log == nil {
		log = slog.Default()
	}
	return &PaymentService{meta: meta, channels: channels, cipher: cipher, cfg: cfg, log: log}
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

	businessID := stringPtrValue(ch.BusinessID)
	wabaID := stringPtrValue(ch.WABAID)
	s.log.Info("payment_link_resolve",
		"channel_id", channelID.String(),
		"team_id", teamID.String(),
		"stored_business_id", businessID,
		"stored_waba_id", wabaID,
		"note", "billing URL uses Meta Business ID; payment status check uses WABA ID",
	)

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

	wabaID := stringPtrValue(ch.WABAID)
	businessID := stringPtrValue(ch.BusinessID)
	log := s.log.With(
		"channel_id", channelID.String(),
		"team_id", teamID.String(),
		"stored_waba_id", wabaID,
		"stored_business_id", businessID,
	)

	if wabaID == "" {
		log.Warn("payment_status_skip",
			"reason", "missing_waba_id",
			"hint", "payment_status must call GET /{WABA_ID} — not Business ID",
		)
		return classifyPaymentStatus("", "", "", wabaID, businessID, "", "", "", false, nil), nil
	}

	if businessID != "" && businessID == wabaID {
		log.Warn("payment_status_id_collision",
			"reason", "stored_waba_id_equals_stored_business_id",
			"hint", "WABA ID and Meta Business ID were likely swapped or duplicated on this channel",
		)
	}

	if s.meta == nil || s.cipher == nil {
		log.Warn("payment_status_skip", "reason", "meta_client_or_cipher_nil")
		return classifyPaymentStatus("", "", "", wabaID, businessID, "", "", "", false, nil), nil
	}
	creds, err := s.channels.FindCredentials(ctx, channelID)
	if err != nil {
		log.Warn("payment_status_skip", "reason", "credentials_unavailable", "error", err.Error())
		return classifyPaymentStatus("", "", "", wabaID, businessID, "", "", "", false, nil), nil
	}
	token, err := s.cipher.Decrypt(creds.AccessTokenEnc)
	if err != nil || strings.TrimSpace(token) == "" {
		log.Warn("payment_status_skip", "reason", "token_decrypt_failed")
		return classifyPaymentStatus("", "", "", wabaID, businessID, "", "", "", false, nil), nil
	}

	log.Info("payment_status_meta_request",
		"meta_endpoint", "GET /{WABA_ID}",
		"meta_fields", "status,business_verification_status,owner_business_info",
		"requested_id_kind", "waba_id",
		"requested_id", wabaID,
		"note", "primary_funding_id is fetched in a separate call",
	)

	waba, err := s.meta.GetWABA(ctx, wabaID, token)
	if err != nil || waba == nil {
		errMsg := "nil_response"
		if err != nil {
			errMsg = err.Error()
		}
		log.Warn("payment_status_meta_error",
			"requested_id_kind", "waba_id",
			"requested_id", wabaID,
			"error", errMsg,
			"hint", "if this ID is actually a Meta Business ID, GetWABA will fail",
		)
		return classifyPaymentStatus("", "", "", wabaID, businessID, "", "", "", false, nil), nil
	}

	ownerBusinessID := ""
	if waba.OwnerBusinessInfo != nil {
		ownerBusinessID = strings.TrimSpace(waba.OwnerBusinessInfo.ID)
	}
	metaObjectID := strings.TrimSpace(waba.ID)
	bizVerif := strings.TrimSpace(waba.BusinessVerificationStatus)

	if metaObjectID != "" && metaObjectID != wabaID {
		log.Warn("payment_status_meta_id_mismatch",
			"requested_waba_id", wabaID,
			"meta_object_id", metaObjectID,
			"hint", "Meta returned a different object id than the stored WABA ID",
		)
	}
	if businessID != "" && ownerBusinessID != "" && businessID != ownerBusinessID {
		log.Warn("payment_status_business_id_mismatch",
			"owner_business_id", ownerBusinessID,
			"hint", "stored Business ID does not match WABA owner_business_info.id from Meta",
		)
	}
	if businessID != "" && businessID == metaObjectID {
		log.Warn("payment_status_used_business_as_waba",
			"meta_object_id", metaObjectID,
			"hint", "stored business_id equals Meta object id returned from GetWABA — IDs may be swapped",
		)
	}

	// Prefer Business node verification_status when Business ID is available.
	if businessID != "" && businessID != wabaID {
		if biz, bizErr := s.meta.GetBusiness(ctx, businessID, token); bizErr != nil {
			log.Warn("business_verification_meta_error",
				"requested_id_kind", "business_id",
				"requested_id", businessID,
				"error", bizErr.Error(),
				"hint", "GET /{BUSINESS_ID} failed — do not use WABA ID for this call",
			)
		} else if biz != nil {
			log.Info("business_verification_meta_response",
				"requested_id_kind", "business_id",
				"requested_id", businessID,
				"meta_business_id", biz.ID,
				"business_verification_status_node", biz.VerificationStatus,
				"waba_business_verification_status", waba.BusinessVerificationStatus,
			)
			if v := strings.TrimSpace(biz.VerificationStatus); v != "" {
				bizVerif = v
			}
		}
	}

	fundingID := ""
	fundingErr := ""
	fundingUnavailable := false
	log.Info("payment_funding_meta_request",
		"meta_endpoint", "GET /{WABA_ID}",
		"meta_fields", "primary_funding_id",
		"requested_id_kind", "waba_id",
		"requested_id", wabaID,
	)
	fundingID, ferr := s.meta.GetWABAPrimaryFunding(ctx, wabaID, token)
	if ferr != nil {
		fundingErr = ferr.Error()
		fundingUnavailable = isPrimaryFundingPermissionError(fundingErr)
		log.Warn("payment_funding_meta_error",
			"requested_id", wabaID,
			"error", fundingErr,
			"bsp_permission_error", fundingUnavailable,
			"hint", "Tech Provider tokens cannot read primary_funding_id; falling back to health_status",
		)
	} else {
		log.Info("payment_funding_meta_response",
			"requested_id", wabaID,
			"has_primary_funding", strings.TrimSpace(fundingID) != "",
		)
	}

	var health *metapkg.WABAHealthStatus
	// Tech Provider path: when funding ID is unreadable, use messaging health_status (error 141006 = payment).
	if strings.TrimSpace(fundingID) == "" {
		log.Info("payment_health_meta_request",
			"meta_endpoint", "GET /{WABA_ID}",
			"meta_fields", "health_status",
			"requested_id", wabaID,
		)
		if hs, herr := s.meta.GetWABAHealthStatus(ctx, wabaID, token); herr != nil {
			log.Warn("payment_health_meta_error", "requested_id", wabaID, "error", herr.Error())
		} else {
			health = hs
			log.Info("payment_health_meta_response",
				"requested_id", wabaID,
				"can_send_message", hs.CanSendMessage,
				"payment_error", healthHasPaymentError(hs),
			)
		}
	}

	result := classifyPaymentStatus(
		fundingID,
		waba.Status,
		bizVerif,
		wabaID,
		businessID,
		metaObjectID,
		ownerBusinessID,
		fundingErr,
		fundingUnavailable,
		health,
	)

	log.Info("payment_status_result",
		"requested_waba_id", result.RequestedWABAID,
		"meta_object_id", result.MetaObjectID,
		"owner_business_id", result.OwnerBusinessID,
		"waba_status", result.WABAStatus,
		"business_verification_status", result.BusinessVerificationStatus,
		"has_primary_funding", result.HasPrimaryFunding,
		"primary_funding_id", result.PrimaryFundingID,
		"funding_check_error", result.FundingCheckError,
		"check_source", result.CheckSource,
		"health_can_send_message", result.HealthCanSendMessage,
		"health_payment_error_code", result.HealthPaymentErrorCode,
		"payment_status", result.Status,
	)

	return result, nil
}

// classifyPaymentStatus maps Meta WABA fields to payment UI state.
// Prefer primary_funding_id when readable (BSP). Otherwise use health_status
// (Tech Provider): error 141006 = payment missing/invalid.
func classifyPaymentStatus(
	primaryFundingID, wabaStatus, businessVerificationStatus, requestedWABAID, storedBusinessID, metaObjectID, ownerBusinessID, fundingCheckError string,
	fundingUnavailable bool,
	health *metapkg.WABAHealthStatus,
) *PaymentStatusResult {
	fundingID := strings.TrimSpace(primaryFundingID)
	status := strings.TrimSpace(wabaStatus)
	bizVerif := strings.TrimSpace(businessVerificationStatus)
	hasFunding := fundingID != ""

	result := &PaymentStatusResult{
		WABAStatus:                 status,
		BusinessVerificationStatus: bizVerif,
		HasPrimaryFunding:          hasFunding,
		FundingCheckError:          strings.TrimSpace(fundingCheckError),
		RequestedWABAID:            strings.TrimSpace(requestedWABAID),
		StoredBusinessID:           strings.TrimSpace(storedBusinessID),
		MetaObjectID:               strings.TrimSpace(metaObjectID),
		OwnerBusinessID:            strings.TrimSpace(ownerBusinessID),
	}
	if hasFunding {
		result.PrimaryFundingID = fundingID
		result.CheckSource = PaymentCheckSourceFunding
	}
	if health != nil {
		result.HealthCanSendMessage = strings.TrimSpace(health.CanSendMessage)
		if code, desc := healthPaymentError(health); code != 0 {
			result.HealthPaymentErrorCode = code
			result.HealthPaymentError = desc
		}
	}

	if isWABARestricted(status) {
		result.Status = PaymentStatusRestricted
		result.Message = PaymentMessageRestricted
		return result
	}
	if hasFunding {
		result.Status = PaymentStatusConfigured
		result.Message = PaymentMessageOK
		return result
	}

	// Tech Provider fallback via health_status.
	if health != nil {
		result.CheckSource = PaymentCheckSourceHealth
		if code, desc := healthPaymentError(health); code != 0 {
			result.Status = PaymentStatusMissing
			if desc != "" {
				result.Message = desc
			} else {
				result.Message = PaymentMessageMissing
			}
			return result
		}
		canSend := strings.ToUpper(strings.TrimSpace(health.CanSendMessage))
		if canSend == "AVAILABLE" || canSend == "LIMITED" {
			result.Status = PaymentStatusConfigured
			result.Message = PaymentMessageOK
			return result
		}
		// BLOCKED for non-payment reasons — payment itself may still be OK.
		result.Status = PaymentStatusConfigured
		result.Message = PaymentMessageOK
		return result
	}

	if fundingUnavailable {
		result.Status = PaymentStatusUnavailable
		result.Message = PaymentMessageUnavailable
		return result
	}
	result.Status = PaymentStatusMissing
	result.Message = PaymentMessageMissing
	return result
}

func healthHasPaymentError(health *metapkg.WABAHealthStatus) bool {
	code, _ := healthPaymentError(health)
	return code != 0
}

func healthPaymentError(health *metapkg.WABAHealthStatus) (int, string) {
	if health == nil {
		return 0, ""
	}
	for _, ent := range health.Entities {
		for _, e := range ent.Errors {
			if e.ErrorCode == metapkg.HealthErrorPaymentMethod {
				desc := strings.TrimSpace(e.ErrorDescription)
				if desc == "" {
					desc = PaymentMessageMissing
				}
				return e.ErrorCode, desc
			}
		}
	}
	return 0, ""
}

func isPrimaryFundingPermissionError(errMsg string) bool {
	msg := strings.ToLower(errMsg)
	return strings.Contains(msg, "business solution provider") ||
		strings.Contains(msg, `"code":10`) ||
		strings.Contains(msg, "(#10)")
}

func isWABARestricted(status string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(status))
	return normalized == "RESTRICTED"
}

func IsBusinessVerified(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "verified")
}

func stringPtrValue(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func defaultPaymentSetupURL(ch *channel.Channel) string {
	// Billing hub is scoped to Meta Business Manager (Business ID), not WABA ID.
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
