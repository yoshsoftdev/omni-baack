package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultAPIVersion = "v21.0"

type Client struct {
	apiVersion   string
	httpClient   *http.Client
	graphBaseURL string
}

func NewClient(apiVersion string) *Client {
	if apiVersion == "" {
		apiVersion = defaultAPIVersion
	}
	return &Client{
		apiVersion: apiVersion,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetBaseURL overrides the Graph API host (used in tests).
func (c *Client) SetBaseURL(baseURL string) {
	c.graphBaseURL = strings.TrimRight(baseURL, "/")
}

func (c *Client) graphURL(path string) string {
	if c.graphBaseURL != "" {
		return c.graphBaseURL + path
	}
	return "https://graph.facebook.com" + path
}

type PhoneNumber struct {
	ID                  string `json:"id"`
	DisplayPhoneNumber  string `json:"display_phone_number"`
	VerifiedName        string `json:"verified_name"`
	QualityRating       string `json:"quality_rating"`
}

func (c *Client) GetPhoneNumber(ctx context.Context, phoneNumberID, accessToken string) (*PhoneNumber, error) {
	url := fmt.Sprintf(
		"https://graph.facebook.com/%s/%s?fields=display_phone_number,verified_name,quality_rating",
		c.apiVersion,
		phoneNumberID,
	)

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

	var phone PhoneNumber
	if err := json.Unmarshal(body, &phone); err != nil {
		return nil, fmt.Errorf("decode meta response: %w", err)
	}
	phone.ID = phoneNumberID
	return &phone, nil
}

type sendMessageRequest struct {
	MessagingProduct string `json:"messaging_product"`
	To               string `json:"to"`
	Type             string `json:"type"`
	Context          *struct {
		MessageID string `json:"message_id"`
	} `json:"context,omitempty"`
	Text             struct {
		Body string `json:"body"`
	} `json:"text"`
}

type sendMessageResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

func (c *Client) SendTextMessage(ctx context.Context, phoneNumberID, accessToken, to, body string) (string, error) {
	return c.SendTextMessageWithContext(ctx, phoneNumberID, accessToken, to, body, "")
}

func (c *Client) SendTextMessageWithContext(ctx context.Context, phoneNumberID, accessToken, to, body, replyToWAMID string) (string, error) {
	payload := sendMessageRequest{
		MessagingProduct: "whatsapp",
		To:               to,
		Type:             "text",
	}
	payload.Text.Body = body
	replyToWAMID = strings.TrimSpace(replyToWAMID)
	if replyToWAMID != "" {
		payload.Context = &struct {
			MessageID string `json:"message_id"`
		}{
			MessageID: replyToWAMID,
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://graph.facebook.com/%s/%s/messages", c.apiVersion, phoneNumberID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("meta send message: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("meta send error: %s", strings.TrimSpace(string(respBody)))
	}

	var result sendMessageResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decode send response: %w", err)
	}
	if len(result.Messages) == 0 {
		return "", fmt.Errorf("meta send: empty message id")
	}
	return result.Messages[0].ID, nil
}

func (c *Client) DeleteMessage(ctx context.Context, phoneNumberID, accessToken, wamid string) error {
	phoneNumberID = strings.TrimSpace(phoneNumberID)
	wamid = strings.TrimSpace(wamid)
	if phoneNumberID == "" || wamid == "" {
		return fmt.Errorf("phone number id and wamid are required")
	}
	url := fmt.Sprintf("https://graph.facebook.com/%s/%s/messages?message_id=%s", c.apiVersion, phoneNumberID, wamid)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("meta delete message: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("meta delete message error: %s", strings.TrimSpace(string(body)))
	}
	return nil
}
