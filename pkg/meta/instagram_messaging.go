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
	MessagingType string `json:"messaging_type,omitempty"`
	Message       any    `json:"message"`
	ReplyTo       *struct {
		MID string `json:"mid"`
	} `json:"reply_to,omitempty"`
}

type instagramSendMessageResponse struct {
	RecipientID string `json:"recipient_id"`
	MessageID   string `json:"message_id"`
}

func (c *Client) instagramMessagesEndpoint(igUserID string) string {
	endpoint := fmt.Sprintf("https://graph.instagram.com/%s/%s/messages", c.apiVersion, url.PathEscape(igUserID))
	if c.graphBaseURL != "" {
		endpoint = fmt.Sprintf("%s/%s/messages", c.graphBaseURL, url.PathEscape(igUserID))
	}
	return endpoint
}

func (c *Client) sendInstagramMessagePayload(ctx context.Context, endpoint, accessToken string, payload any) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
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

// SendInstagramTextMessage sends a DM from an Instagram professional account to a scoped user (IGSID).
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
	payload.Message = map[string]string{"text": body}
	replyToMID = strings.TrimSpace(replyToMID)
	if replyToMID != "" {
		payload.MessagingType = "RESPONSE"
		payload.ReplyTo = &struct {
			MID string `json:"mid"`
		}{MID: replyToMID}
	}
	return c.sendInstagramMessagePayload(ctx, c.instagramMessagesEndpoint(igUserID), accessToken, payload)
}

func normalizeInstagramMediaType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image":
		return "image"
	case "video":
		return "video"
	case "audio":
		return "audio"
	case "document", "file":
		return "file"
	default:
		return ""
	}
}

// instagramMediaMessageBody builds the message object for Instagram Login send.
// Always use singular "attachment" + payload.url. The docs sample that uses
// plural "attachments" with url is rejected by graph.instagram.com
// ("Invalid keys \"url\" were found in param \"message[attachments][0]\"").
func instagramMediaMessageBody(mediaType, mediaURL string) map[string]any {
	return map[string]any{
		"attachment": map[string]any{
			"type": mediaType,
			"payload": map[string]any{
				"url": mediaURL,
			},
		},
	}
}

// ValidateInstagramOutboundMIME rejects formats Instagram Messaging cannot fetch/send.
// Meta error 2018047 is commonly type/URL mismatch (e.g. image/webp labeled as image).
func ValidateInstagramOutboundMIME(mimeType string) error {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	switch mimeType {
	case "image/jpeg", "image/jpg", "image/png",
		"video/mp4", "video/ogg", "video/avi", "video/quicktime", "video/webm",
		"audio/aac", "audio/mp4", "audio/mpeg", "audio/wav", "audio/x-wav", "audio/wave",
		"application/pdf":
		return nil
	case "image/webp":
		return fmt.Errorf("instagram does not support WebP for outbound media; use JPEG or PNG")
	case "video/3gpp":
		return fmt.Errorf("instagram does not support 3GPP video; use MP4, MOV, WebM, OGG, or AVI")
	default:
		if mimeType == "" {
			return fmt.Errorf("instagram media mime type is required")
		}
		return fmt.Errorf("instagram does not support mime type %s", mimeType)
	}
}

// SendInstagramMediaMessage sends media by public URL via Instagram Login Messaging API.
// Meta fetches the URL server-side; multipart filedata is not supported on graph.instagram.com.
func (c *Client) SendInstagramMediaMessage(
	ctx context.Context,
	igUserID, accessToken, recipientScopedID, contentType, mediaURL, caption, replyToMID string,
) (string, error) {
	igUserID = strings.TrimSpace(igUserID)
	recipientScopedID = strings.TrimSpace(recipientScopedID)
	mediaType := normalizeInstagramMediaType(contentType)
	mediaURL = strings.TrimSpace(mediaURL)
	caption = strings.TrimSpace(caption)
	replyToMID = strings.TrimSpace(replyToMID)
	if igUserID == "" {
		return "", fmt.Errorf("instagram user id is required")
	}
	if recipientScopedID == "" {
		return "", fmt.Errorf("instagram recipient id is required")
	}
	if mediaType == "" {
		return "", fmt.Errorf("instagram media type is unsupported")
	}
	if mediaURL == "" {
		return "", fmt.Errorf("instagram media url is required")
	}
	if err := validatePublicMediaURL(mediaURL); err != nil {
		return "", err
	}

	payload := instagramSendMessageRequest{}
	payload.Recipient.ID = recipientScopedID
	payload.Message = instagramMediaMessageBody(mediaType, mediaURL)
	if replyToMID != "" {
		payload.MessagingType = "RESPONSE"
		payload.ReplyTo = &struct {
			MID string `json:"mid"`
		}{MID: replyToMID}
	}
	mid, err := c.sendInstagramMessagePayload(ctx, c.instagramMessagesEndpoint(igUserID), accessToken, payload)
	if err != nil {
		return "", err
	}
	if caption != "" {
		// Instagram Login media send does not include caption in the attachment payload.
		_, _ = c.SendInstagramTextMessage(ctx, igUserID, accessToken, recipientScopedID, caption, "")
	}
	return mid, nil
}

func validatePublicMediaURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("instagram media url is invalid")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && scheme != "http" {
		return fmt.Errorf("instagram media url must be http(s)")
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasSuffix(host, ".local") {
		return fmt.Errorf("instagram media url must be publicly reachable (not localhost). Set WEBHOOK_PUBLIC_BASE_URL to the public HTTPS webhook-service base")
	}
	return nil
}
