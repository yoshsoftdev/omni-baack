package automation

import (
	"encoding/json"
	"testing"
)

func TestParseAndValidateSendTemplateConfig(t *testing.T) {
	raw, _ := json.Marshal(SendTemplateActionConfig{
		TemplateID:    "00000000-0000-0000-0000-000000000001",
		TemplateName:  "invoice",
		RecipientMode: RecipientModePhone,
		VariableMappings: []VariableMapping{
			{TemplateVariable: "1", APIParameter: "customerName", Required: true},
		},
	})
	cfg, err := parseAndValidateSendTemplateConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TemplateName != "invoice" {
		t.Fatalf("unexpected %#v", cfg)
	}

	_, err = parseAndValidateSendTemplateConfig(json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestParseAndValidateSendTextConfig(t *testing.T) {
	raw, _ := json.Marshal(SendTextActionConfig{
		ChannelID:     "00000000-0000-0000-0000-000000000123",
		RecipientMode: RecipientModePhone,
		TextParameter: " text ",
	})
	cfg, err := parseAndValidateSendTextConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TextParameter != "text" {
		t.Fatalf("unexpected text parameter: %q", cfg.TextParameter)
	}

	_, err = parseAndValidateSendTextConfig(json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestNormalizeScopesAllowsSendText(t *testing.T) {
	scopes := normalizeScopes([]string{" send_text ", "unknown", "send_template", "send_text"})
	if len(scopes) != 2 {
		t.Fatalf("unexpected scopes length: %#v", scopes)
	}
	if scopes[0] != "send_text" || scopes[1] != "send_template" {
		t.Fatalf("unexpected scopes: %#v", scopes)
	}
}

func TestParseWebhookConfig(t *testing.T) {
	cfg, err := parseWebhookConfig(&WebhookConfig{
		URL:       " https://hooks.example.com/omni ",
		IsEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != "https://hooks.example.com/omni" {
		t.Fatalf("unexpected url: %q", cfg.URL)
	}
	if cfg.Secret == "" {
		t.Fatal("expected auto-generated secret")
	}

	_, err = parseWebhookConfig(&WebhookConfig{URL: "", IsEnabled: true})
	if err == nil {
		t.Fatal("expected validation error for enabled empty url")
	}

	_, err = parseWebhookConfig(&WebhookConfig{URL: "ftp://hooks.example.com", IsEnabled: true})
	if err == nil {
		t.Fatal("expected validation error for non-http scheme")
	}
}
