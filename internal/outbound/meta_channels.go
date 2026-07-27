package outbound

import (
	"context"
	"fmt"
	"strings"

	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

// MetaClient is the Meta Graph surface used by WhatsAppChannel / InstagramChannel.
// Implemented by *meta.Client.
type MetaClient interface {
	SendTextMessageWithContext(ctx context.Context, phoneNumberID, accessToken, to, body, replyToWAMID string) (string, error)
	SendMediaMessageWithContext(ctx context.Context, phoneNumberID, accessToken, to, messageType, mediaID, caption, filename, replyToWAMID string) (string, error)
	DeleteMessage(ctx context.Context, phoneNumberID, accessToken, wamid string) error
	SendInstagramTextMessage(ctx context.Context, igUserID, accessToken, recipientScopedID, body, replyToMID string) (string, error)
	SendInstagramMediaMessage(ctx context.Context, igUserID, accessToken, recipientScopedID, contentType, mediaURL, caption, replyToMID string) (string, error)
}

// WhatsAppChannel delivers WhatsApp Cloud API messages via Meta.
type WhatsAppChannel struct {
	meta MetaClient
}

// NewWhatsAppChannel wraps a Meta client as a WhatsApp outbound Channel.
func NewWhatsAppChannel(meta MetaClient) *WhatsAppChannel {
	return &WhatsAppChannel{meta: meta}
}

func (c *WhatsAppChannel) SendText(ctx context.Context, creds Credentials, msg TextMessage) (string, error) {
	if c == nil || c.meta == nil {
		return "", fmt.Errorf("whatsapp channel is not configured")
	}
	phoneNumberID := strings.TrimSpace(creds.PhoneNumberID)
	if phoneNumberID == "" {
		return "", fmt.Errorf("whatsapp phone_number_id is required")
	}
	return c.meta.SendTextMessageWithContext(
		ctx,
		phoneNumberID,
		creds.AccessToken,
		strings.TrimSpace(msg.To),
		msg.Body,
		strings.TrimSpace(msg.ReplyToProviderID),
	)
}

func (c *WhatsAppChannel) SendMedia(ctx context.Context, creds Credentials, msg MediaMessage) (string, error) {
	if c == nil || c.meta == nil {
		return "", fmt.Errorf("whatsapp channel is not configured")
	}
	phoneNumberID := strings.TrimSpace(creds.PhoneNumberID)
	if phoneNumberID == "" {
		return "", fmt.Errorf("whatsapp phone_number_id is required")
	}
	mediaID := strings.TrimSpace(msg.ProviderMediaID)
	if mediaID == "" {
		return "", fmt.Errorf("whatsapp provider media id is required")
	}
	return c.meta.SendMediaMessageWithContext(
		ctx,
		phoneNumberID,
		creds.AccessToken,
		strings.TrimSpace(msg.To),
		strings.TrimSpace(msg.Type),
		mediaID,
		msg.Caption,
		msg.Filename,
		strings.TrimSpace(msg.ReplyToProviderID),
	)
}

func (c *WhatsAppChannel) DeleteMessage(ctx context.Context, creds Credentials, providerMessageID string) error {
	if c == nil || c.meta == nil {
		return nil
	}
	phoneNumberID := strings.TrimSpace(creds.PhoneNumberID)
	wamid := strings.TrimSpace(providerMessageID)
	if phoneNumberID == "" || wamid == "" {
		return nil
	}
	return c.meta.DeleteMessage(ctx, phoneNumberID, creds.AccessToken, wamid)
}

// InstagramChannel delivers Instagram Messaging API messages via Meta.
type InstagramChannel struct {
	meta MetaClient
}

// NewInstagramChannel wraps a Meta client as an Instagram outbound Channel.
func NewInstagramChannel(meta MetaClient) *InstagramChannel {
	return &InstagramChannel{meta: meta}
}

func (c *InstagramChannel) SendText(ctx context.Context, creds Credentials, msg TextMessage) (string, error) {
	if c == nil || c.meta == nil {
		return "", fmt.Errorf("instagram channel is not configured")
	}
	igUserID := strings.TrimSpace(creds.IGUserID)
	if igUserID == "" {
		return "", fmt.Errorf("instagram ig_user_id is required")
	}
	return c.meta.SendInstagramTextMessage(
		ctx,
		igUserID,
		creds.AccessToken,
		strings.TrimSpace(msg.To),
		msg.Body,
		strings.TrimSpace(msg.ReplyToProviderID),
	)
}

func (c *InstagramChannel) SendMedia(ctx context.Context, creds Credentials, msg MediaMessage) (string, error) {
	if c == nil || c.meta == nil {
		return "", fmt.Errorf("instagram channel is not configured")
	}
	igUserID := strings.TrimSpace(creds.IGUserID)
	if igUserID == "" {
		return "", fmt.Errorf("instagram ig_user_id is required")
	}
	if err := metapkg.ValidateInstagramOutboundMIME(msg.MIMEType); err != nil {
		return "", err
	}
	mediaURL := strings.TrimSpace(msg.MediaURL)
	if mediaURL == "" {
		return "", fmt.Errorf("instagram media url is required")
	}
	return c.meta.SendInstagramMediaMessage(
		ctx,
		igUserID,
		creds.AccessToken,
		strings.TrimSpace(msg.To),
		strings.TrimSpace(msg.Type),
		mediaURL,
		msg.Caption,
		strings.TrimSpace(msg.ReplyToProviderID),
	)
}

func (c *InstagramChannel) DeleteMessage(context.Context, Credentials, string) error {
	// Instagram Messaging does not support agent unsend via this path.
	return nil
}
