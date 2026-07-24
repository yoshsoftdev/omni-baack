package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type WABAOwnerBusinessInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type WABA struct {
	ID                         string                 `json:"id"`
	Name                       string                 `json:"name"`
	TimezoneID                 string                 `json:"timezone_id"`
	MessageTemplateNamespace   string                 `json:"message_template_namespace"`
	PrimaryFundingID           string                 `json:"primary_funding_id"`
	Status                     string                 `json:"status"`
	BusinessVerificationStatus string                 `json:"business_verification_status"`
	OwnerBusinessInfo          *WABAOwnerBusinessInfo `json:"owner_business_info"`
}

type Business struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	VerificationStatus string `json:"verification_status"`
}

// WABAHealthStatus is Meta messaging eligibility for a WABA (Tech Provider–readable).
// Payment issues surface as entity error_code 141006.
type WABAHealthStatus struct {
	CanSendMessage string                 `json:"can_send_message"`
	Entities       []WABAHealthEntity     `json:"entities"`
}

type WABAHealthEntity struct {
	EntityType     string            `json:"entity_type"`
	ID             string            `json:"id"`
	CanSendMessage string            `json:"can_send_message"`
	Errors         []WABAHealthError `json:"errors"`
	AdditionalInfo []string          `json:"additional_info"`
}

type WABAHealthError struct {
	ErrorCode         int    `json:"error_code"`
	ErrorDescription  string `json:"error_description"`
	PossibleSolution  string `json:"possible_solution"`
}

const (
	// Meta health_status error: payment method missing/invalid on WABA.
	HealthErrorPaymentMethod = 141006
)

func (c *Client) GetWABA(ctx context.Context, wabaID, accessToken string) (*WABA, error) {
	// Do NOT request primary_funding_id here.
	// That field requires the app-owning business to be a WhatsApp BSP; bundling it
	// fails the entire GET and hides fields that system-user tokens can read
	// (status, business_verification_status, owner_business_info).
	endpoint := fmt.Sprintf(
		"https://graph.facebook.com/%s/%s?fields=id,name,timezone_id,message_template_namespace,status,business_verification_status,owner_business_info{id,name}",
		c.apiVersion,
		wabaID,
	)
	return decodeGET[WABA](c, ctx, endpoint, accessToken)
}

// GetWABAPrimaryFunding fetches only primary_funding_id (BSP / billing-sensitive field).
func (c *Client) GetWABAPrimaryFunding(ctx context.Context, wabaID, accessToken string) (string, error) {
	endpoint := fmt.Sprintf(
		"https://graph.facebook.com/%s/%s?fields=primary_funding_id",
		c.apiVersion,
		wabaID,
	)
	out, err := decodeGET[struct {
		PrimaryFundingID string `json:"primary_funding_id"`
	}](c, ctx, endpoint, accessToken)
	if err != nil {
		return "", err
	}
	if out == nil {
		return "", nil
	}
	return strings.TrimSpace(out.PrimaryFundingID), nil
}

// GetWABAHealthStatus fetches messaging health (works for Tech Provider tokens).
func (c *Client) GetWABAHealthStatus(ctx context.Context, wabaID, accessToken string) (*WABAHealthStatus, error) {
	endpoint := fmt.Sprintf(
		"https://graph.facebook.com/%s/%s?fields=health_status",
		c.apiVersion,
		wabaID,
	)
	out, err := decodeGET[struct {
		HealthStatus *WABAHealthStatus `json:"health_status"`
	}](c, ctx, endpoint, accessToken)
	if err != nil {
		return nil, err
	}
	if out == nil || out.HealthStatus == nil {
		return nil, fmt.Errorf("meta api error: health_status missing")
	}
	return out.HealthStatus, nil
}

func (c *Client) GetBusiness(ctx context.Context, businessID, accessToken string) (*Business, error) {
	endpoint := fmt.Sprintf(
		"https://graph.facebook.com/%s/%s?fields=id,name,verification_status",
		c.apiVersion,
		businessID,
	)
	return decodeGET[Business](c, ctx, endpoint, accessToken)
}

func (c *Client) Ping(ctx context.Context, accessToken string) error {
	endpoint := fmt.Sprintf("https://graph.facebook.com/%s/me?fields=id", c.apiVersion)
	_, err := decodeGET[struct {
		ID string `json:"id"`
	}](c, ctx, endpoint, accessToken)
	return err
}

type subscribedAppsResponse struct {
	Success bool `json:"success"`
}

// SubscribeAppToWABA registers this app to receive webhooks for the WABA (required for CoEx).
func (c *Client) SubscribeAppToWABA(ctx context.Context, wabaID, accessToken string) error {
	endpoint := fmt.Sprintf("https://graph.facebook.com/%s/%s/subscribed_apps", c.apiVersion, wabaID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader("{}"))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("subscribe app to waba: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		if IsAlreadySubscribed(resp.StatusCode, string(body)) {
			return nil
		}
		return fmt.Errorf("subscribe app to waba error: %s", strings.TrimSpace(string(body)))
	}

	var result subscribedAppsResponse
	if err := json.Unmarshal(body, &result); err == nil && !result.Success {
		return fmt.Errorf("subscribe app to waba error: success=false")
	}
	return nil
}

func decodeGET[T any](c *Client, ctx context.Context, url, accessToken string) (*T, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("meta api request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("meta api error: %s", strings.TrimSpace(string(body)))
	}

	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode meta response: %w", err)
	}
	return &out, nil
}
