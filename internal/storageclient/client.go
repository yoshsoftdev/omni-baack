package storageclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/google/uuid"
)

type StoredObject struct {
	ID            uuid.UUID `json:"id"`
	TeamID        uuid.UUID `json:"team_id"`
	ObjectKey     string    `json:"object_key"`
	StorageDriver string    `json:"storage_driver"`
	MimeType      string    `json:"mime_type"`
	SizeBytes     int64     `json:"size_bytes"`
	Hash          string    `json:"hash"`
}

type apiResponse struct {
	Success bool `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Object StoredObject `json:"object"`
	} `json:"data"`
}

type Client struct {
	baseURL       string
	internalToken string
	httpClient    *http.Client
}

func New(baseURL, internalToken string) *Client {
	return &Client{
		baseURL:       strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		internalToken: strings.TrimSpace(internalToken),
		httpClient:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) GetMetadata(ctx context.Context, teamID, objectID uuid.UUID) (*StoredObject, error) {
	url := fmt.Sprintf("%s/api/v1/storage/objects/%s", c.baseURL, objectID.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.applyInternalHeaders(req, teamID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("storage metadata request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("storage object not found")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("storage metadata error: %s", strings.TrimSpace(string(body)))
	}

	var envelope apiResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode storage metadata: %w", err)
	}
	if envelope.Data.Object.ID == uuid.Nil {
		return nil, fmt.Errorf("storage metadata missing object")
	}
	return &envelope.Data.Object, nil
}

func (c *Client) Download(ctx context.Context, teamID, objectID uuid.UUID) (io.ReadCloser, *StoredObject, error) {
	meta, err := c.GetMetadata(ctx, teamID, objectID)
	if err != nil {
		return nil, nil, err
	}
	url := fmt.Sprintf("%s/api/v1/storage/objects/%s/content", c.baseURL, objectID.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	c.applyInternalHeaders(req, teamID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("storage download request: %w", err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("storage download error: %s", strings.TrimSpace(string(body)))
	}
	return resp.Body, meta, nil
}

func (c *Client) Upload(ctx context.Context, teamID uuid.UUID, namespace, filename, mimeType string, body io.Reader) (*StoredObject, error) {
	if strings.TrimSpace(namespace) == "" {
		namespace = "attachments"
	}
	if strings.TrimSpace(filename) == "" {
		filename = "attachment"
	}
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	_ = writer.WriteField("namespace", namespace)

	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	fileHeader := textproto.MIMEHeader{}
	fileHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	fileHeader.Set("Content-Type", mimeType)
	part, err := writer.CreatePart(fileHeader)
	if err != nil {
		return nil, fmt.Errorf("storage upload create part: %w", err)
	}
	if _, err := io.Copy(part, body); err != nil {
		return nil, fmt.Errorf("storage upload copy body: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("storage upload close writer: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/storage/upload", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &payload)
	if err != nil {
		return nil, err
	}
	c.applyInternalHeaders(req, teamID)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("storage upload request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("storage upload error: %s", strings.TrimSpace(string(respBody)))
	}

	var envelope apiResponse
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("decode storage upload: %w", err)
	}
	if envelope.Data.Object.ID == uuid.Nil {
		return nil, fmt.Errorf("storage upload missing object")
	}
	return &envelope.Data.Object, nil
}

func (c *Client) applyInternalHeaders(req *http.Request, teamID uuid.UUID) {
	req.Header.Set("X-Internal-Token", c.internalToken)
	req.Header.Set("X-Team-ID", teamID.String())
}
