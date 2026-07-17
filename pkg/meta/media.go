package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path/filepath"
	"strings"
)

type uploadMediaResponse struct {
	ID string `json:"id"`
}

type MediaMetadata struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}

func (c *Client) UploadMedia(ctx context.Context, phoneNumberID, accessToken, mimeType, filename string, body io.Reader, size int64) (string, error) {
	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" {
		return "", fmt.Errorf("mime type is required")
	}
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "attachment"
	} else {
		filename = filepath.Base(filename)
	}

	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	_ = writer.WriteField("messaging_product", "whatsapp")

	fileHeader := textproto.MIMEHeader{}
	fileHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	fileHeader.Set("Content-Type", mimeType)
	part, err := writer.CreatePart(fileHeader)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, body); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/%s/%s/media", c.graphURL(""), c.apiVersion, phoneNumberID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &payload)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("meta upload media: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("meta upload error: %s", strings.TrimSpace(string(respBody)))
	}

	var result uploadMediaResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decode upload response: %w", err)
	}
	if strings.TrimSpace(result.ID) == "" {
		return "", fmt.Errorf("meta upload: empty media id")
	}
	_ = size
	return result.ID, nil
}

func (c *Client) SendMediaMessage(ctx context.Context, phoneNumberID, accessToken, to, messageType, mediaID, caption, filename string) (string, error) {
	return c.SendMediaMessageWithContext(ctx, phoneNumberID, accessToken, to, messageType, mediaID, caption, filename, "")
}

func (c *Client) SendMediaMessageWithContext(
	ctx context.Context,
	phoneNumberID, accessToken, to, messageType, mediaID, caption, filename, replyToWAMID string,
) (string, error) {
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              messageType,
	}
	replyToWAMID = strings.TrimSpace(replyToWAMID)
	if replyToWAMID != "" {
		payload["context"] = map[string]string{
			"message_id": replyToWAMID,
		}
	}
	media := map[string]string{"id": mediaID}
	if strings.TrimSpace(caption) != "" {
		media["caption"] = caption
	}
	if messageType == "document" && strings.TrimSpace(filename) != "" {
		media["filename"] = filename
	}
	payload[messageType] = media

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/%s/%s/messages", c.graphURL(""), c.apiVersion, phoneNumberID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("meta send media message: %w", err)
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

func (c *Client) GetMediaMetadata(ctx context.Context, mediaID, phoneNumberID, accessToken string) (*MediaMetadata, error) {
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return nil, fmt.Errorf("media id is required")
	}
	query := ""
	if strings.TrimSpace(phoneNumberID) != "" {
		query = "?phone_number_id=" + phoneNumberID
	}
	url := fmt.Sprintf("%s/%s/%s%s", c.graphURL(""), c.apiVersion, mediaID, query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("meta get media metadata: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("meta get media metadata error: %s", strings.TrimSpace(string(respBody)))
	}
	var result MediaMetadata
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode media metadata: %w", err)
	}
	if strings.TrimSpace(result.URL) == "" {
		return nil, fmt.Errorf("meta media metadata missing url")
	}
	return &result, nil
}

func (c *Client) DownloadMedia(ctx context.Context, mediaURL, accessToken string) (io.ReadCloser, error) {
	mediaURL = strings.TrimSpace(mediaURL)
	if mediaURL == "" {
		return nil, fmt.Errorf("media url is required")
	}
	if !IsAllowedMetaMediaURL(mediaURL) {
		return nil, fmt.Errorf("media url host is not allowed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("meta download media: %w", err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("meta download media error: %s", strings.TrimSpace(string(body)))
	}
	return resp.Body, nil
}

// IsAllowedMetaMediaURL returns true for Meta/Instagram CDN and Graph media hosts.
func IsAllowedMetaMediaURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && scheme != "http" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return false
	}
	allowedExact := map[string]struct{}{
		"lookaside.fbsbx.com":       {},
		"lookaside.instagram.com":   {},
		"scontent.cdninstagram.com": {},
		"graph.facebook.com":        {},
		"graph.instagram.com":       {},
	}
	if _, ok := allowedExact[host]; ok {
		return true
	}
	if strings.HasSuffix(host, ".fbcdn.net") ||
		strings.HasSuffix(host, ".cdninstagram.com") ||
		strings.HasSuffix(host, ".fbsbx.com") {
		return true
	}
	return false
}
