package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type instagramSendMessageRequest struct {
	Recipient struct {
		ID string `json:"id"`
	} `json:"recipient"`
	Message struct {
		Text string `json:"text"`
	} `json:"message"`
	ReplyTo *struct {
		MID string `json:"mid"`
	} `json:"reply_to,omitempty"`
}

type instagramSendMessageResponse struct {
	RecipientID string `json:"recipient_id"`
	MessageID   string `json:"message_id"`
}

// SendInstagramTextMessage sends a DM from an Instagram professional account to a scoped user (IGSID).
// replyToMID is accepted for forward-compat but is intentionally not sent to Meta:
// Instagram Login messaging frequently returns error_subcode 2534002 ("Invalid message ID") for reply_to.
func (c *Client) SendInstagramTextMessage(ctx context.Context, igUserID, accessToken, recipientScopedID, body, replyToMID string) (string, error) {
	igUserID = strings.TrimSpace(igUserID)
	recipientScopedID = strings.TrimSpace(recipientScopedID)
	body = strings.TrimSpace(body)
	if igUserID == "" {
		return "", fmt.Errorf("instagram user id is required")
	}
	if recipientScopedID == "" {
		return "", fmt.Errorf("instagram recipient id is required")
	}
	if body == "" {
		return "", fmt.Errorf("message text is required")
	}

	payload := instagramSendMessageRequest{}
	payload.Recipient.ID = recipientScopedID
	payload.Message.Text = body
	_ = replyToMID

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	endpoint := fmt.Sprintf("https://graph.instagram.com/%s/%s/messages", c.apiVersion, url.PathEscape(igUserID))
	if c.graphBaseURL != "" {
		endpoint = fmt.Sprintf("%s/%s/messages", c.graphBaseURL, url.PathEscape(igUserID))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("instagram send message: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	safeBody := redactSensitiveJSON(respBody)
	if resp.StatusCode >= 400 {
		return "", NewAPIError(resp.StatusCode, safeBody)
	}

	var result instagramSendMessageResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decode instagram send response: %w", err)
	}
	if strings.TrimSpace(result.MessageID) == "" {
		return "", fmt.Errorf("instagram send: empty message id")
	}
	return result.MessageID, nil
}
