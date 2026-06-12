package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type WABAPhoneNumbersResponse struct {
	Data []PhoneNumber `json:"data"`
}

// ExchangeEmbeddedCode exchanges the authorization code from Embedded Signup for an access token.
// When using the Facebook JS SDK with response_type code, redirect_uri must be omitted.
func (c *Client) ExchangeEmbeddedCode(ctx context.Context, appID, appSecret, code string) (*TokenResponse, error) {
	endpoint := fmt.Sprintf("https://graph.facebook.com/%s/oauth/access_token", c.apiVersion)
	params := url.Values{
		"client_id":     {appID},
		"client_secret": {appSecret},
		"code":          {code},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchange embedded code: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("exchange embedded code error: %s", strings.TrimSpace(string(body)))
	}

	var token TokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if token.AccessToken == "" {
		return nil, fmt.Errorf("empty access token in response")
	}
	return &token, nil
}

func (c *Client) ListWABAPhoneNumbers(ctx context.Context, wabaID, accessToken string) ([]PhoneNumber, error) {
	endpoint := fmt.Sprintf(
		"https://graph.facebook.com/%s/%s/phone_numbers?fields=id,display_phone_number,verified_name,quality_rating",
		c.apiVersion,
		wabaID,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list waba phone numbers: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("list waba phone numbers error: %s", strings.TrimSpace(string(body)))
	}

	var result WABAPhoneNumbersResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode waba phone numbers: %w", err)
	}
	return result.Data, nil
}

type SMBSyncResponse struct {
	MessagingProduct string `json:"messaging_product"`
	RequestID        string `json:"request_id"`
}

// RequestSMBSync triggers contact or history sync for coexistence onboarding.
// syncType must be "smb_app_state_sync" or "history".
func (c *Client) RequestSMBSync(ctx context.Context, phoneNumberID, accessToken, syncType string) (*SMBSyncResponse, error) {
	endpoint := fmt.Sprintf("https://graph.facebook.com/%s/%s/smb_app_data", c.apiVersion, phoneNumberID)
	payload := map[string]string{
		"messaging_product": "whatsapp",
		"sync_type":         syncType,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request smb sync: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("request smb sync error: %s", strings.TrimSpace(string(body)))
	}

	var result SMBSyncResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode smb sync response: %w", err)
	}
	return &result, nil
}

type CoexistenceStatus struct {
	IsOnBizApp   bool   `json:"is_on_biz_app"`
	PlatformType string `json:"platform_type"`
	ID           string `json:"id"`
}

func (c *Client) GetCoexistenceStatus(ctx context.Context, phoneNumberID, accessToken string) (*CoexistenceStatus, error) {
	endpoint := fmt.Sprintf(
		"https://graph.facebook.com/%s/%s?fields=is_on_biz_app,platform_type",
		c.apiVersion,
		phoneNumberID,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get coexistence status: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("get coexistence status error: %s", strings.TrimSpace(string(body)))
	}

	var status CoexistenceStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, fmt.Errorf("decode coexistence status: %w", err)
	}
	return &status, nil
}

func ParseUnixTimestamp(raw string) time.Time {
	if raw == "" {
		return time.Now().UTC()
	}
	var ts int64
	if _, err := fmt.Sscanf(raw, "%d", &ts); err != nil {
		return time.Now().UTC()
	}
	return time.Unix(ts, 0).UTC()
}
