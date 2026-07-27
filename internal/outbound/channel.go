// Package outbound defines provider Channel.Send adapters for Inbox delivery.
// Message domain depends on these interfaces — never on pkg/meta send APIs.
package outbound

import "context"

// Credentials are decrypted channel credentials for a single send.
type Credentials struct {
	AccessToken   string
	PhoneNumberID string // WhatsApp Cloud phone_number_id
	IGUserID      string // Instagram Login ig_user_id
}

// TextMessage is a free-form text outbound payload.
type TextMessage struct {
	To                  string
	Body                string
	ReplyToProviderID   string
}

// MediaMessage is an outbound media payload.
// WhatsApp uses ProviderMediaID (Cloud API media id); Instagram uses MediaURL.
type MediaMessage struct {
	To                string
	Type              string // image | video | audio | document
	ProviderMediaID   string
	MediaURL          string
	Caption           string
	Filename          string
	ReplyToProviderID string
	MIMEType          string // used for Instagram MIME validation before send
}

// Channel sends messages through a connected provider channel (WhatsApp or Instagram).
type Channel interface {
	SendText(ctx context.Context, creds Credentials, msg TextMessage) (providerMessageID string, err error)
	SendMedia(ctx context.Context, creds Credentials, msg MediaMessage) (providerMessageID string, err error)
	DeleteMessage(ctx context.Context, creds Credentials, providerMessageID string) error
}
