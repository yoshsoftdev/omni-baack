package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/pkg/cryptoutil"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

// MetaSender sends WhatsApp template messages via Meta Cloud API.
type MetaSender interface {
	SendTemplateMessage(
		ctx context.Context,
		phoneNumberID, accessToken, to, templateName, language string,
		components []metaclient.TemplateComponent,
	) (string, error)
}

// SendInput carries everything required to send one template message.
type SendInput struct {
	Campaign  *campaign.Campaign
	Template  *template.Template
	Channel   *channel.Channel
	Recipient *campaign.Recipient
	Contact   *contact.Contact
}

// SendResult is a successful Meta send response.
type SendResult struct {
	WAMID            string
	ProviderResponse json.RawMessage
}

// Sender renders variables and calls Meta Cloud API.
type Sender struct {
	meta     MetaSender
	channels ChannelStore
	cipher   *cryptoutil.TokenCipher
	metrics  *Metrics
	log      *slog.Logger
}

func NewSender(meta MetaSender, channels ChannelStore, cipher *cryptoutil.TokenCipher, metrics *Metrics, log *slog.Logger) *Sender {
	if log == nil {
		log = slog.Default()
	}
	return &Sender{meta: meta, channels: channels, cipher: cipher, metrics: metrics, log: log}
}

func (s *Sender) Send(ctx context.Context, in SendInput) (*SendResult, error) {
	if in.Channel.PhoneNumberID == nil || *in.Channel.PhoneNumberID == "" {
		return nil, fmt.Errorf("channel missing phone_number_id")
	}

	log := s.log.With(
		"campaign_id", in.Campaign.ID.String(),
		"recipient_id", in.Recipient.ID.String(),
		"template_id", in.Template.ID.String(),
		"channel_id", in.Channel.ID.String(),
		"phone_number_id", PhoneNumberID(in.Channel),
	)

	mapping, err := campaign.ParseVariableMapping(in.Campaign.VariableMapping)
	if err != nil {
		if s.metrics != nil {
			s.metrics.IncTemplateRenderFailed(in.Channel.ID)
		}
		return nil, err
	}
	samples := campaign.ParseSampleValues(in.Campaign.SampleValues)

	contactEntity := in.Contact
	if contactEntity == nil {
		contactEntity = &contact.Contact{Name: "", Phone: in.Recipient.Phone}
	}

	components, err := campaign.RenderValidatedTemplate(in.Template, mapping, contactEntity, samples)
	if err != nil {
		if campaign.IsSendValidationError(err) {
			if s.metrics != nil {
				s.metrics.IncTemplateValidationFailed(in.Channel.ID)
			}
			log.Warn("template validation failed", "error", err)
			return nil, err
		}
		if s.metrics != nil {
			s.metrics.IncTemplateRenderFailed(in.Channel.ID)
		}
		return nil, err
	}
	if s.metrics != nil {
		s.metrics.IncTemplateRenderSuccess(in.Channel.ID)
		recordMediaSendMetrics(s.metrics, in.Channel.ID, mapping)
	}

	creds, err := s.channels.FindCredentials(ctx, in.Channel.ID)
	if err != nil {
		return nil, err
	}
	token, err := s.cipher.Decrypt(creds.AccessTokenEnc)
	if err != nil {
		return nil, fmt.Errorf("decrypt channel token: %w", err)
	}

	sendStart := time.Now()
	wamid, err := s.meta.SendTemplateMessage(
		ctx,
		*in.Channel.PhoneNumberID,
		token,
		in.Recipient.Phone,
		in.Template.Name,
		in.Template.Language,
		components,
	)
	if err != nil {
		return nil, err
	}
	if s.metrics != nil {
		s.metrics.ObserveMetaSendDuration(time.Since(sendStart))
	}

	raw, _ := json.Marshal(map[string]string{"wamid": wamid})
	log.Info("template sent")
	return &SendResult{WAMID: wamid, ProviderResponse: raw}, nil
}

// PhoneNumberID returns the Meta phone number id for logging.
func PhoneNumberID(ch *channel.Channel) string {
	if ch == nil || ch.PhoneNumberID == nil {
		return ""
	}
	return *ch.PhoneNumberID
}

// ChannelID is a helper for metrics keyed by channel.
func ChannelID(ch *channel.Channel) uuid.UUID {
	if ch == nil {
		return uuid.Nil
	}
	return ch.ID
}

func recordMediaSendMetrics(metrics *Metrics, channelID uuid.UUID, mapping campaign.VariableMapping) {
	if metrics == nil || mapping.Header == nil {
		return
	}
	ref := mapping.Header.MediaReference()
	switch {
	case ref.UsesMediaID():
		metrics.IncMediaIDSend(channelID)
	case ref.UsesLink():
		metrics.IncMediaLinkSend(channelID)
	}
}
