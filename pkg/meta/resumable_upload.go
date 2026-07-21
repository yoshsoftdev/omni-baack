package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type startUploadSessionResponse struct {
	ID string `json:"id"`
}

type finishUploadResponse struct {
	Handle string `json:"h"`
}

// UploadResumableMedia uploads sample media via Meta App Resumable Upload API
// and returns the asset handle required for WhatsApp template HEADER examples.
func (c *Client) UploadResumableMedia(
	ctx context.Context,
	appID, accessToken, fileName, mimeType string,
	body io.Reader,
	sizeBytes int64,
) (string, error) {
	appID = strings.TrimSpace(appID)
	accessToken = strings.TrimSpace(accessToken)
	fileName = strings.TrimSpace(fileName)
	mimeType = strings.TrimSpace(mimeType)
	if appID == "" {
		return "", fmt.Errorf("meta app id is required for resumable upload")
	}
	if accessToken == "" {
		return "", fmt.Errorf("access token is required for resumable upload")
	}
	if sizeBytes <= 0 {
		return "", fmt.Errorf("file length must be greater than zero")
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if fileName == "" {
		fileName = "sample"
	}
	fileName = path.Base(fileName)

	sessionID, err := c.startResumableUploadSession(ctx, appID, accessToken, fileName, mimeType, sizeBytes)
	if err != nil {
		return "", err
	}
	return c.uploadResumableFile(ctx, sessionID, accessToken, body, sizeBytes)
}

func (c *Client) startResumableUploadSession(
	ctx context.Context,
	appID, accessToken, fileName, mimeType string,
	sizeBytes int64,
) (string, error) {
	query := url.Values{}
	query.Set("file_length", fmt.Sprintf("%d", sizeBytes))
	query.Set("file_type", mimeType)
	query.Set("file_name", fileName)

	endpoint := c.graphURL(fmt.Sprintf("/%s/%s/uploads?%s", c.apiVersion, url.PathEscape(appID), query.Encode()))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.uploadHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("meta start upload session: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", NewAPIError(resp.StatusCode, string(respBody))
	}

	var result startUploadSessionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decode upload session: %w", err)
	}
	sessionID := strings.TrimSpace(result.ID)
	if sessionID == "" {
		return "", fmt.Errorf("meta upload session id is empty")
	}
	return sessionID, nil
}

func (c *Client) uploadResumableFile(
	ctx context.Context,
	sessionID, accessToken string,
	body io.Reader,
	sizeBytes int64,
) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", fmt.Errorf("upload session id is required")
	}

	payload, err := io.ReadAll(io.LimitReader(body, sizeBytes+1))
	if err != nil {
		return "", fmt.Errorf("read upload body: %w", err)
	}
	if int64(len(payload)) != sizeBytes {
		return "", fmt.Errorf("upload body size mismatch: got %d want %d", len(payload), sizeBytes)
	}

	endpoint := c.graphURL(fmt.Sprintf("/%s/%s", c.apiVersion, sessionID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "OAuth "+accessToken)
	req.Header.Set("file_offset", "0")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = sizeBytes

	resp, err := c.uploadHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("meta upload file: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", NewAPIError(resp.StatusCode, string(respBody))
	}

	var result finishUploadResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decode upload handle: %w", err)
	}
	handle := strings.TrimSpace(result.Handle)
	if handle == "" {
		return "", fmt.Errorf("meta upload handle is empty")
	}
	return handle, nil
}

func (c *Client) uploadHTTPClient() *http.Client {
	if c.httpClient != nil && c.httpClient.Timeout >= 2*time.Minute {
		return c.httpClient
	}
	return &http.Client{Timeout: 2 * time.Minute}
}
