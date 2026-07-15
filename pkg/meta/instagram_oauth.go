package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// InstagramBusinessLoginScopes are permissions for Instagram API with Instagram Login.
const InstagramBusinessLoginScopes = "instagram_business_basic,instagram_business_manage_messages,instagram_business_manage_comments,instagram_business_content_publish"

// InstagramWebhookSubscribeFields are webhook fields requested when subscribing the app to an IG account.
const InstagramWebhookSubscribeFields = "messages,messaging_seen,messaging_postbacks,messaging_referral,message_reactions,comments"

const (
	instagramAccountTypeBusiness = "BUSINESS"
	instagramAccountTypeCreator  = "MEDIA_CREATOR"
	instagramAccountTypePersonal = "PERSONAL"
)

// InstagramUser is the authenticated Instagram professional account profile.
type InstagramUser struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	AccountType string `json:"account_type"`
}

// InstagramCodeExchangeResponse is returned by POST api.instagram.com/oauth/access_token.
type InstagramCodeExchangeResponse struct {
	AccessToken string `json:"access_token"`
	UserID      string `json:"user_id"`
	Permissions string `json:"permissions"`
}

type instagramCodeExchangeEnvelope struct {
	Data []json.RawMessage `json:"data"`
}

// BuildInstagramAuthorizeURL builds the Instagram Business Login authorize URL.
func BuildInstagramAuthorizeURL(appID, redirectURI, state, scope string) string {
	if scope == "" {
		scope = InstagramBusinessLoginScopes
	}
	params := url.Values{
		"client_id":     {appID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {scope},
		"state":         {state},
		"force_reauth":  {"true"},
	}
	return "https://www.instagram.com/oauth/authorize?" + params.Encode()
}

// ExchangeInstagramCode exchanges an authorization code for a short-lived Instagram user token.
func (c *Client) ExchangeInstagramCode(ctx context.Context, appID, appSecret, redirectURI, code string) (*InstagramCodeExchangeResponse, error) {
	code = strings.TrimSuffix(strings.TrimSpace(code), "#_")
	endpoint := "https://api.instagram.com/oauth/access_token"
	form := url.Values{
		"client_id":     {appID},
		"client_secret": {appSecret},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
		"code":          {code},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	slog.Info("instagram oauth: exchanging authorization code",
		"endpoint", endpoint,
		"client_id", appID,
		"redirect_uri", redirectURI,
		"code_len", len(code),
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchange instagram code: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	safeBody := redactSensitiveJSON(body)
	slog.Info("instagram oauth: code exchange response",
		"status", resp.StatusCode,
		"body", safeBody,
		"body_len", len(body),
	)

	if resp.StatusCode >= 400 {
		return nil, NewAPIError(resp.StatusCode, safeBody)
	}

	parsed, err := parseInstagramCodeExchange(body)
	if err != nil {
		slog.Error("instagram oauth: failed to parse code exchange response",
			"status", resp.StatusCode,
			"body", safeBody,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("empty access token in instagram response (status=%d body=%s)", resp.StatusCode, safeBody)
	}
	slog.Info("instagram oauth: short-lived token received",
		"user_id", parsed.UserID,
		"has_permissions", strings.TrimSpace(parsed.Permissions) != "",
	)
	return parsed, nil
}

// ExchangeInstagramLongLivedToken exchanges a short-lived Instagram token for a long-lived token (~60 days).
func (c *Client) ExchangeInstagramLongLivedToken(ctx context.Context, appSecret, shortLivedToken string) (*TokenResponse, error) {
	params := url.Values{
		"grant_type":    {"ig_exchange_token"},
		"client_secret": {appSecret},
		"access_token":  {shortLivedToken},
	}
	endpoint := "https://graph.instagram.com/access_token?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	slog.Info("instagram oauth: exchanging long-lived token", "endpoint", "https://graph.instagram.com/access_token", "method", http.MethodGet)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchange instagram long-lived token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	safeBody := redactSensitiveJSON(body)
	slog.Info("instagram oauth: long-lived token response",
		"status", resp.StatusCode,
		"body", safeBody,
		"body_len", len(body),
	)

	if resp.StatusCode >= 400 {
		return nil, NewAPIError(resp.StatusCode, safeBody)
	}

	var token TokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("decode instagram long-lived token response: %w; body=%s", err, safeBody)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return nil, fmt.Errorf("empty long-lived instagram access token (body=%s)", safeBody)
	}
	slog.Info("instagram oauth: long-lived token received", "expires_in", token.ExpiresIn)
	return &token, nil
}

// SubscribeInstagramApp registers this app to receive Instagram webhooks for the professional account.
// Docs: POST https://graph.instagram.com/{ig-user-id}/subscribed_apps
func (c *Client) SubscribeInstagramApp(ctx context.Context, igUserID, accessToken string, fields string) error {
	igUserID = strings.TrimSpace(igUserID)
	if igUserID == "" {
		return fmt.Errorf("instagram user id is required for webhook subscribe")
	}
	if fields == "" {
		fields = InstagramWebhookSubscribeFields
	}

	params := url.Values{"subscribed_fields": {fields}}
	endpoint := fmt.Sprintf(
		"https://graph.instagram.com/%s/%s/subscribed_apps?%s",
		c.apiVersion,
		url.PathEscape(igUserID),
		params.Encode(),
	)
	if c.graphBaseURL != "" {
		endpoint = fmt.Sprintf("%s/%s/subscribed_apps?%s", c.graphBaseURL, url.PathEscape(igUserID), params.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader("{}"))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	slog.Info("instagram oauth: subscribing app webhooks",
		"ig_user_id", igUserID,
		"fields", fields,
		"endpoint", strings.Split(endpoint, "?")[0],
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("subscribe instagram app: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	safeBody := redactSensitiveJSON(body)
	slog.Info("instagram oauth: subscribe webhooks response",
		"status", resp.StatusCode,
		"body", safeBody,
		"ig_user_id", igUserID,
	)

	if resp.StatusCode >= 400 {
		if IsAlreadySubscribed(resp.StatusCode, string(body)) {
			return nil
		}
		return NewAPIError(resp.StatusCode, safeBody)
	}

	var result subscribedAppsResponse
	if err := json.Unmarshal(body, &result); err == nil && !result.Success {
		return fmt.Errorf("subscribe instagram app error: success=false body=%s", safeBody)
	}
	return nil
}

// GetInstagramUser loads the authenticated Instagram account profile.
func (c *Client) GetInstagramUser(ctx context.Context, accessToken string) (*InstagramUser, error) {
	endpoint := fmt.Sprintf("https://graph.instagram.com/me?fields=id,user_id,username,name,account_type")
	if c.graphBaseURL != "" {
		endpoint = c.graphBaseURL + "/me?fields=id,user_id,username,name,account_type"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get instagram user: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, NewAPIError(resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var user InstagramUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("decode instagram user response: %w", err)
	}
	if strings.TrimSpace(user.UserID) == "" {
		user.UserID = strings.TrimSpace(user.ID)
	}
	return &user, nil
}

// InstagramScopedUser is a messaging participant profile (IGSID).
type InstagramScopedUser struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	Name       string `json:"name"`
	ProfilePic string `json:"profile_pic"`
}

// GetInstagramScopedUser loads profile info for a user who messaged the business account.
func (c *Client) GetInstagramScopedUser(ctx context.Context, scopedUserID, accessToken string) (*InstagramScopedUser, error) {
	scopedUserID = strings.TrimSpace(scopedUserID)
	if scopedUserID == "" {
		return nil, fmt.Errorf("instagram scoped user id is required")
	}
	endpoint := fmt.Sprintf(
		"https://graph.instagram.com/%s/%s?fields=id,username,name,profile_pic",
		c.apiVersion,
		url.PathEscape(scopedUserID),
	)
	if c.graphBaseURL != "" {
		endpoint = fmt.Sprintf("%s/%s?fields=id,username,name,profile_pic", c.graphBaseURL, url.PathEscape(scopedUserID))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get instagram scoped user: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	safeBody := redactSensitiveJSON(body)
	if resp.StatusCode >= 400 {
		return nil, NewAPIError(resp.StatusCode, safeBody)
	}

	var user InstagramScopedUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("decode instagram scoped user: %w", err)
	}
	return &user, nil
}

// ValidateInstagramAccountType rejects personal Instagram accounts.
func ValidateInstagramAccountType(accountType string) error {
	normalized := strings.ToUpper(strings.TrimSpace(accountType))
	switch normalized {
	case instagramAccountTypeBusiness, instagramAccountTypeCreator:
		return nil
	case instagramAccountTypePersonal:
		return fmt.Errorf("personal Instagram accounts are not supported; convert to a Business or Creator account")
	case "":
		return fmt.Errorf("instagram account type is missing")
	default:
		return fmt.Errorf("unsupported instagram account type %q; only Business and Creator accounts are supported", accountType)
	}
}

// TokenExpiryFromExpiresIn converts Meta expires_in seconds to an absolute time.
func TokenExpiryFromExpiresIn(expiresIn int) *time.Time {
	if expiresIn <= 0 {
		return nil
	}
	t := time.Now().UTC().Add(time.Duration(expiresIn) * time.Second)
	return &t
}

func parseInstagramCodeExchange(body []byte) (*InstagramCodeExchangeResponse, error) {
	if parsed, ok := decodeInstagramTokenObject(body); ok {
		return parsed, nil
	}

	var wrapped instagramCodeExchangeEnvelope
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, fmt.Errorf("decode instagram token response: %w", err)
	}
	if len(wrapped.Data) == 0 {
		return nil, fmt.Errorf("instagram token response data is empty")
	}
	if parsed, ok := decodeInstagramTokenObject(wrapped.Data[0]); ok {
		return parsed, nil
	}
	return nil, fmt.Errorf("instagram token response missing access_token")
}

func decodeInstagramTokenObject(raw []byte) (*InstagramCodeExchangeResponse, bool) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, false
	}
	token := strings.TrimSpace(jsonRawAsString(obj["access_token"]))
	if token == "" {
		return nil, false
	}
	return &InstagramCodeExchangeResponse{
		AccessToken: token,
		UserID:      strings.TrimSpace(jsonRawAsString(obj["user_id"])),
		Permissions: strings.TrimSpace(jsonRawAsString(obj["permissions"])),
	}, true
}

func jsonRawAsString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	var asNumber json.Number
	if err := json.Unmarshal(raw, &asNumber); err == nil {
		return asNumber.String()
	}
	var asList []string
	if err := json.Unmarshal(raw, &asList); err == nil {
		return strings.Join(asList, ",")
	}
	var asAny []any
	if err := json.Unmarshal(raw, &asAny); err == nil {
		parts := make([]string, 0, len(asAny))
		for _, item := range asAny {
			parts = append(parts, fmt.Sprint(item))
		}
		return strings.Join(parts, ",")
	}
	return strings.Trim(string(raw), `"`)
}

func redactSensitiveJSON(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()
	var anyJSON any
	if err := dec.Decode(&anyJSON); err != nil {
		return truncated(trimmed, 512)
	}
	redactSensitiveValue(anyJSON)
	out, err := json.Marshal(anyJSON)
	if err != nil {
		return truncated(trimmed, 512)
	}
	return string(out)
}

func redactSensitiveValue(v any) {
	switch typed := v.(type) {
	case map[string]any:
		for key, child := range typed {
			switch strings.ToLower(key) {
			case "access_token", "accessToken", "token", "client_secret", "code":
				if s, ok := child.(string); ok && s != "" {
					typed[key] = redactedPreview(s)
				} else {
					typed[key] = "[redacted]"
				}
			default:
				redactSensitiveValue(child)
			}
		}
	case []any:
		for _, child := range typed {
			redactSensitiveValue(child)
		}
	}
}

func redactedPreview(value string) string {
	if len(value) <= 8 {
		return "[redacted]"
	}
	return value[:4] + "…" + strconv.Itoa(len(value)) + "chars"
}

func truncated(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
