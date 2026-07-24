package meta

import (
	"context"

	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

// MetaClient is the unified abstraction for Meta Graph API operations.
type MetaClient interface {
	VerifyWebhook(mode, token, challenge, expectedVerifyToken string) (string, error)
	ListTemplates(ctx context.Context, wabaID, accessToken string) ([]MessageTemplate, error)
	GetPhoneNumbers(ctx context.Context, wabaID, accessToken string) ([]PhoneNumber, error)
	GetPhoneNumber(ctx context.Context, phoneNumberID, accessToken string) (*PhoneNumber, error)
	GetWABA(ctx context.Context, wabaID, accessToken string) (*WABA, error)
	GetWABAPrimaryFunding(ctx context.Context, wabaID, accessToken string) (string, error)
	GetWABAHealthStatus(ctx context.Context, wabaID, accessToken string) (*metapkg.WABAHealthStatus, error)
	GetBusiness(ctx context.Context, businessID, accessToken string) (*Business, error)
	Ping(ctx context.Context, accessToken string) error
	ExchangeEmbeddedCode(ctx context.Context, appID, appSecret, code string) (*TokenResponse, error)
	SubscribeAppToWABA(ctx context.Context, wabaID, accessToken string) error
}

type MessageTemplate = metapkg.MessageTemplate
type PhoneNumber = metapkg.PhoneNumber
type WABA = metapkg.WABA
type Business = metapkg.Business
type TokenResponse = metapkg.TokenResponse

// GraphClient adapts pkg/meta.Client to MetaClient.
type GraphClient struct {
	inner *metapkg.Client
}

func NewGraphClient(inner *metapkg.Client) *GraphClient {
	return &GraphClient{inner: inner}
}

func (c *GraphClient) VerifyWebhook(mode, token, challenge, expectedVerifyToken string) (string, error) {
	return metapkg.VerifyWebhookSubscription(mode, token, challenge, expectedVerifyToken)
}

func (c *GraphClient) ListTemplates(ctx context.Context, wabaID, accessToken string) ([]MessageTemplate, error) {
	return c.inner.ListMessageTemplates(ctx, wabaID, accessToken)
}

func (c *GraphClient) GetPhoneNumbers(ctx context.Context, wabaID, accessToken string) ([]PhoneNumber, error) {
	return c.inner.ListWABAPhoneNumbers(ctx, wabaID, accessToken)
}

func (c *GraphClient) GetPhoneNumber(ctx context.Context, phoneNumberID, accessToken string) (*PhoneNumber, error) {
	return c.inner.GetPhoneNumber(ctx, phoneNumberID, accessToken)
}

func (c *GraphClient) GetWABA(ctx context.Context, wabaID, accessToken string) (*WABA, error) {
	return c.inner.GetWABA(ctx, wabaID, accessToken)
}

func (c *GraphClient) GetWABAPrimaryFunding(ctx context.Context, wabaID, accessToken string) (string, error) {
	return c.inner.GetWABAPrimaryFunding(ctx, wabaID, accessToken)
}

func (c *GraphClient) GetWABAHealthStatus(ctx context.Context, wabaID, accessToken string) (*metapkg.WABAHealthStatus, error) {
	return c.inner.GetWABAHealthStatus(ctx, wabaID, accessToken)
}

func (c *GraphClient) GetBusiness(ctx context.Context, businessID, accessToken string) (*Business, error) {
	return c.inner.GetBusiness(ctx, businessID, accessToken)
}

func (c *GraphClient) Ping(ctx context.Context, accessToken string) error {
	return c.inner.Ping(ctx, accessToken)
}

func (c *GraphClient) ExchangeEmbeddedCode(ctx context.Context, appID, appSecret, code string) (*TokenResponse, error) {
	return c.inner.ExchangeEmbeddedCode(ctx, appID, appSecret, code)
}

func (c *GraphClient) SubscribeAppToWABA(ctx context.Context, wabaID, accessToken string) error {
	return c.inner.SubscribeAppToWABA(ctx, wabaID, accessToken)
}
