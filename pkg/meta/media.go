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
	"path/filepath"
	"strings"
)

type uploadMediaResponse struct {
	ID string `json:"id"`
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
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              messageType,
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
