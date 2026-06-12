package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type MessageTemplate struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Language   string          `json:"language"`
	Status     string          `json:"status"`
	Category   string          `json:"category"`
	Components json.RawMessage `json:"components"`
}

type TemplateParameter struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type TemplateComponent struct {
	Type       string              `json:"type"`
	SubType    string              `json:"sub_type,omitempty"`
	Index      string              `json:"index,omitempty"`
	Parameters []TemplateParameter `json:"parameters,omitempty"`
}

const (
	OTPTemplateName     = "kode_otp"
	OTPTemplateLanguage = "id"
)

// BuildKodeOTPComponents builds Meta template components for the kode_otp authentication template.
func BuildKodeOTPComponents(code string) []TemplateComponent {
	param := TemplateParameter{Type: "text", Text: code}
	return []TemplateComponent{
		{
			Type:       "body",
			Parameters: []TemplateParameter{param},
		},
		{
			Type:       "button",
			SubType:    "url",
			Index:      "0",
			Parameters: []TemplateParameter{param},
		},
	}
}

type listTemplatesResponse struct {
	Data   []MessageTemplate `json:"data"`
	Paging struct {
		Next string `json:"next"`
	} `json:"paging"`
}

func (c *Client) ListMessageTemplates(ctx context.Context, wabaID, accessToken string) ([]MessageTemplate, error) {
	var all []MessageTemplate
	url := fmt.Sprintf(
		"https://graph.facebook.com/%s/%s/message_templates?fields=id,name,language,status,category,components&limit=100",
		c.apiVersion,
		wabaID,
	)

	for url != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("meta list templates: %w", err)
		}

		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("meta list templates error: %s", strings.TrimSpace(string(body)))
		}

		var result listTemplatesResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("decode templates: %w", err)
		}
		all = append(all, result.Data...)
		url = result.Paging.Next
	}

	return all, nil
}

func (c *Client) SendTemplateMessage(
	ctx context.Context,
	phoneNumberID, accessToken, to, templateName, language string,
	components []TemplateComponent,
) (string, error) {
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "template",
		"template": map[string]any{
			"name": templateName,
			"language": map[string]string{
				"code": language,
			},
		},
	}
	if len(components) > 0 {
		payload["template"].(map[string]any)["components"] = components
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
		return "", fmt.Errorf("meta send template: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("meta send template error: %s", strings.TrimSpace(string(respBody)))
	}

	var result sendMessageResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decode template send response: %w", err)
	}
	if len(result.Messages) == 0 {
		return "", fmt.Errorf("meta send template: empty message id")
	}
	return result.Messages[0].ID, nil
}

type CreateTemplateRequest struct {
	Name       string          `json:"name"`
	Language   string          `json:"language"`
	Category   string          `json:"category"`
	Components json.RawMessage `json:"components"`
}

type CreateTemplateResponse struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Category string `json:"category"`
}

func (c *Client) CreateMessageTemplate(
	ctx context.Context,
	wabaID, accessToken string,
	input CreateTemplateRequest,
) (*CreateTemplateResponse, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://graph.facebook.com/%s/%s/message_templates", c.apiVersion, wabaID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("meta create template: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("meta create template error: %s", strings.TrimSpace(string(respBody)))
	}

	var result CreateTemplateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode create template response: %w", err)
	}
	return &result, nil
}
