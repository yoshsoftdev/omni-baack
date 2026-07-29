package automation

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type EndpointStatus string

const (
	EndpointStatusActive   EndpointStatus = "active"
	EndpointStatusDisabled EndpointStatus = "disabled"
)

type ActionType string

const (
	ActionSendTemplate ActionType = "send_template"
	ActionSendText     ActionType = "send_text"
)

type RecipientMode string

const (
	RecipientModePhone RecipientMode = "phone"
)

type VariableMapping struct {
	TemplateVariable string `json:"templateVariable"`
	APIParameter     string `json:"apiParameter"`
	Required         bool   `json:"required"`
	DefaultValue     string `json:"defaultValue,omitempty"`
	Description      string `json:"description,omitempty"`
}

type SendTemplateActionConfig struct {
	TemplateID       string            `json:"templateId"`
	TemplateName     string            `json:"templateName"`
	RecipientMode    RecipientMode     `json:"recipientMode"`
	VariableMappings []VariableMapping `json:"variableMappings"`
}

type SendTextActionConfig struct {
	ChannelID     string        `json:"channelId"`
	RecipientMode RecipientMode `json:"recipientMode"`
	TextParameter string        `json:"textParameter"`
	DefaultText   string        `json:"defaultText,omitempty"`
}

type EndpointStats struct {
	TotalCalls       int64   `json:"totalCalls"`
	SuccessRate      float64 `json:"successRate"`
	FailedCalls      int64   `json:"failedCalls"`
	CallsToday       int64   `json:"callsToday"`
	AverageLatencyMs int64   `json:"averageLatencyMs"`
}

type Endpoint struct {
	ID           string          `json:"id"`
	TeamID       uuid.UUID       `json:"teamId"`
	Name         string          `json:"name"`
	Slug         string          `json:"slug"`
	Description  string          `json:"description"`
	Method       string          `json:"method"`
	Status       EndpointStatus  `json:"status"`
	ActionType   ActionType      `json:"actionType"`
	ActionConfig json.RawMessage `json:"actionConfig"`
	Webhook      WebhookConfig   `json:"webhook"`
	CreatedBy    *uuid.UUID      `json:"createdBy,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
	DeletedAt    *time.Time      `json:"deletedAt,omitempty"`
	LastUsedAt   *time.Time      `json:"lastUsedAt,omitempty"`
	Stats        EndpointStats   `json:"stats"`
}

type WebhookConfig struct {
	URL       string `json:"url"`
	IsEnabled bool   `json:"isEnabled"`
	Secret    string `json:"secret,omitempty"`
}

type CreateEndpointInput struct {
	Name         string
	Slug         string
	Description  string
	ActionType   ActionType
	ActionConfig json.RawMessage
	Webhook      *WebhookConfig
}

type UpdateEndpointInput struct {
	Name         *string
	Slug         *string
	Description  *string
	ActionConfig json.RawMessage
	Status       *EndpointStatus
	Webhook      *WebhookConfig
}

type EndpointListFilter struct {
	Search     string
	Status     string
	ActionType string
}

type APIKeyStatus string

const (
	APIKeyStatusActive   APIKeyStatus = "active"
	APIKeyStatusDisabled APIKeyStatus = "disabled"
)

type APIKey struct {
	ID         string       `json:"id"`
	TeamID     uuid.UUID    `json:"teamId"`
	Name       string       `json:"name"`
	KeyPrefix  string       `json:"keyPrefix"`
	Scopes     []string     `json:"scopes"`
	Status     APIKeyStatus `json:"status"`
	CreatedBy  *uuid.UUID   `json:"createdBy,omitempty"`
	CreatedAt  time.Time    `json:"createdAt"`
	UpdatedAt  time.Time    `json:"updatedAt"`
	DeletedAt  *time.Time   `json:"deletedAt,omitempty"`
	LastUsedAt *time.Time   `json:"lastUsedAt,omitempty"`
	// KeyHash is internal-only and never serialized.
	KeyHash string `json:"-"`
}

type CreateAPIKeyInput struct {
	Name   string
	Scopes []string
}

type APIKeySecretResult struct {
	Key    *APIKey `json:"key"`
	Secret string  `json:"secret"`
}

type Summary struct {
	EndpointCount int64   `json:"endpointCount"`
	RequestsToday int64   `json:"requestsToday"`
	SuccessRate   float64 `json:"successRate"`
	FailedToday   int64   `json:"failedToday"`
	APIKeyCount   int64   `json:"apiKeyCount"`
}

type ExecutionStatus string

const (
	ExecutionQueued  ExecutionStatus = "queued"
	ExecutionSuccess ExecutionStatus = "success"
	ExecutionFailed  ExecutionStatus = "failed"
)

type Execution struct {
	ID                string          `json:"id"`
	TeamID            uuid.UUID       `json:"teamId"`
	EndpointID        string          `json:"endpointId"`
	APIKeyID          *string         `json:"apiKeyId,omitempty"`
	Status            ExecutionStatus `json:"status"`
	StatusCode        int             `json:"statusCode"`
	LatencyMs         int             `json:"latencyMs"`
	ErrorMessage      *string         `json:"errorMessage,omitempty"`
	ClientIP          *string         `json:"clientIp,omitempty"`
	ProviderMessageID *string         `json:"providerMessageId,omitempty"`
	CreatedAt         time.Time       `json:"createdAt"`
}

type WebhookDeliveryStatus string

const (
	WebhookDeliverySuccess WebhookDeliveryStatus = "success"
	WebhookDeliveryFailed  WebhookDeliveryStatus = "failed"
)

type WebhookDelivery struct {
	ID           string                `json:"id"`
	TeamID       uuid.UUID             `json:"teamId"`
	EndpointID   string                `json:"endpointId"`
	ExecutionID  string                `json:"executionId"`
	Status       WebhookDeliveryStatus `json:"status"`
	StatusCode   int                   `json:"statusCode"`
	ErrorMessage *string               `json:"errorMessage,omitempty"`
	AttemptedAt  time.Time             `json:"attemptedAt"`
}

type EndpointAnalytics struct {
	EndpointID       string           `json:"endpointId"`
	CallsToday       int64            `json:"callsToday"`
	TotalRequests    int64            `json:"totalRequests"`
	SuccessRate      float64          `json:"successRate"`
	FailedRequests   int64            `json:"failedRequests"`
	AverageLatencyMs int64            `json:"averageLatencyMs"`
	LastUsedAt       *time.Time       `json:"lastUsedAt,omitempty"`
	LastError        *LastError       `json:"lastError,omitempty"`
	TopClientIPs     []ClientIPCount  `json:"topClientIps"`
	RequestsOverTime []DayRequestStat `json:"requestsOverTime"`
}

type LastError struct {
	At         time.Time `json:"at"`
	Message    string    `json:"message"`
	StatusCode int       `json:"statusCode"`
}

type ClientIPCount struct {
	IP    string `json:"ip"`
	Count int64  `json:"count"`
}

type DayRequestStat struct {
	Date    string `json:"date"`
	Success int64  `json:"success"`
	Failed  int64  `json:"failed"`
}

type InvokeResult struct {
	Success     bool           `json:"success"`
	StatusCode  int            `json:"statusCode"`
	LatencyMs   int            `json:"latencyMs"`
	Body        map[string]any `json:"body"`
	ExecutionID string         `json:"executionId,omitempty"`
	MessageID   string         `json:"messageId,omitempty"`
}

// Forward webhook — pushes inbound Omni Chan messages to an external app.

const EventMessageReceived = "message.received"

type ForwardWebhookStatus string

const (
	ForwardWebhookActive   ForwardWebhookStatus = "active"
	ForwardWebhookDisabled ForwardWebhookStatus = "disabled"
)

type ForwardWebhook struct {
	ID                       string               `json:"id"`
	TeamID                   uuid.UUID            `json:"teamId"`
	Name                     string               `json:"name"`
	URL                      string               `json:"url"`
	Secret                   string               `json:"secret,omitempty"`
	Status                   ForwardWebhookStatus `json:"status"`
	Events                   []string             `json:"events"`
	ChannelID                *uuid.UUID           `json:"channelId,omitempty"`
	OnlyUnassigned           bool                 `json:"onlyUnassigned"`
	AssignAutomationOnReopen bool                 `json:"assignAutomationOnReopen"`
	CreatedBy                *uuid.UUID           `json:"createdBy,omitempty"`
	CreatedAt                time.Time            `json:"createdAt"`
	UpdatedAt                time.Time            `json:"updatedAt"`
	DeletedAt                *time.Time           `json:"deletedAt,omitempty"`
	LastUsedAt               *time.Time           `json:"lastUsedAt,omitempty"`
}

type CreateForwardWebhookInput struct {
	Name                     string
	URL                      string
	Secret                   string
	Events                   []string
	ChannelID                *uuid.UUID
	OnlyUnassigned           bool
	AssignAutomationOnReopen bool
}

type UpdateForwardWebhookInput struct {
	Name                     *string
	URL                      *string
	Status                   *ForwardWebhookStatus
	Events                   []string
	ChannelID                *uuid.UUID
	OnlyUnassigned           *bool
	AssignAutomationOnReopen *bool
	ClearChannel             bool
	RotateSecret             bool
	Secret                   *string
}

type ForwardDeliveryStatus string

const (
	ForwardDeliverySuccess ForwardDeliveryStatus = "success"
	ForwardDeliveryFailed  ForwardDeliveryStatus = "failed"
)

type ForwardDelivery struct {
	ID             string                `json:"id"`
	TeamID         uuid.UUID             `json:"teamId"`
	WebhookID      string                `json:"webhookId"`
	EventType      string                `json:"eventType"`
	MessageID      *uuid.UUID            `json:"messageId,omitempty"`
	ConversationID *uuid.UUID            `json:"conversationId,omitempty"`
	Status         ForwardDeliveryStatus `json:"status"`
	StatusCode     int                   `json:"statusCode"`
	ErrorMessage   *string               `json:"errorMessage,omitempty"`
	AttemptedAt    time.Time             `json:"attemptedAt"`
}

// InboundForwardPayload is POSTed to customer apps when Omni Chan receives a message.
type InboundForwardPayload struct {
	Event          string                `json:"event"`
	OccurredAt     time.Time             `json:"occurredAt"`
	TeamID         string                `json:"teamId"`
	ChannelID      string                `json:"channelId"`
	ChannelType    string                `json:"channelType"`
	ConversationID string                `json:"conversationId"`
	Contact        InboundForwardContact `json:"contact"`
	Message        InboundForwardMessage `json:"message"`
}

type InboundForwardContact struct {
	ID           string `json:"id"`
	Phone        string `json:"phone,omitempty"`
	ContactName  string `json:"contactName,omitempty"`
	CustomerName string `json:"customerName,omitempty"`
}

type InboundForwardMessage struct {
	ID          string    `json:"id"`
	WAMID       string    `json:"wamid,omitempty"`
	Direction   string    `json:"direction"`
	ContentType string    `json:"contentType"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"createdAt"`
}

type InboxAutomationSettings struct {
	Enabled    bool `json:"enabled"`
	HasWebhook bool `json:"hasWebhook"`
}
