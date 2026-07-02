package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type WABA struct {
	ID                       string `json:"id"`
	Name                     string `json:"name"`
	TimezoneID               string `json:"timezone_id"`
	MessageTemplateNamespace string `json:"message_template_namespace"`
	PrimaryFundingID         string `json:"primary_funding_id"`
}

type Business struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *Client) GetWABA(ctx context.Context, wabaID, accessToken string) (*WABA, error) {
	endpoint := fmt.Sprintf(
		"https://graph.facebook.com/%s/%s?fields=id,name,timezone_id,message_template_namespace,primary_funding_id",
		c.apiVersion,
		wabaID,
	)
	return decodeGET[WABA](c, ctx, endpoint, accessToken)
}

func (c *Client) GetBusiness(ctx context.Context, businessID, accessToken string) (*Business, error) {
	endpoint := fmt.Sprintf(
		"https://graph.facebook.com/%s/%s?fields=id,name",
		c.apiVersion,
		businessID,
	)
	return decodeGET[Business](c, ctx, endpoint, accessToken)
}

func (c *Client) Ping(ctx context.Context, accessToken string) error {
	endpoint := fmt.Sprintf("https://graph.facebook.com/%s/me?fields=id", c.apiVersion)
	_, err := decodeGET[struct {
		ID string `json:"id"`
	}](c, ctx, endpoint, accessToken)
	return err
}

func decodeGET[T any](c *Client, ctx context.Context, url, accessToken string) (*T, error) {
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

	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode meta response: %w", err)
	}
	return &out, nil
}
