package meta

import (
	"encoding/json"
	"testing"
)

func TestBuildTextHeader(t *testing.T) {
	components := json.RawMessage(`[
		{"type":"HEADER","format":"TEXT","text":"Hi {{1}}"},
		{"type":"BODY","text":"Welcome"}
	]`)
	payload, err := BuildTemplatePayload(components, VariableMappingInput{
		HeaderText: map[string]string{"1": "name"},
	}, func(_, source string) string {
		if source == "name" {
			return "Alice"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(payload.Components))
	}
	if payload.Components[0].Type != "header" {
		t.Fatalf("expected header component")
	}
	if payload.Components[0].Parameters[0].Text != "Alice" {
		t.Fatalf("unexpected header text: %s", payload.Components[0].Parameters[0].Text)
	}
}

func TestBuildImageHeaderMediaURL(t *testing.T) {
	components := json.RawMessage(`[{"type":"HEADER","format":"IMAGE"}]`)
	payload, err := BuildTemplatePayload(components, VariableMappingInput{
		HeaderMedia: MediaReference{Link: "https://cdn.example.com/promo.jpg"},
	}, func(_, _ string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Components) != 1 {
		t.Fatal("expected header component")
	}
	if payload.Components[0].Parameters[0].Image == nil {
		t.Fatal("expected image parameter")
	}
	if payload.Components[0].Parameters[0].Image.Link != "https://cdn.example.com/promo.jpg" {
		t.Fatalf("unexpected image link: %s", payload.Components[0].Parameters[0].Image.Link)
	}
}

func TestBuildURLButtonSubstitution(t *testing.T) {
	components := json.RawMessage(`[{"type":"BUTTONS","buttons":[{"type":"URL","text":"Track","url":"https://shop.example/{{1}}"}]}]`)
	payload, err := BuildTemplatePayload(components, VariableMappingInput{
		Buttons: map[string]ButtonMappingInput{
			"0": {URLVars: map[string]string{"1": "order_id"}},
		},
	}, func(index, source string) string {
		if index == "1" && source == "order_id" {
			return "ORD-42"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Components) != 1 {
		t.Fatal("expected button component")
	}
	if payload.Components[0].SubType != "url" {
		t.Fatalf("expected url button, got %s", payload.Components[0].SubType)
	}
	if payload.Components[0].Parameters[0].Text != "ORD-42" {
		t.Fatalf("unexpected button param: %s", payload.Components[0].Parameters[0].Text)
	}
}

func TestBuildCopyCodeButton(t *testing.T) {
	components := json.RawMessage(`[{"type":"BUTTONS","buttons":[{"type":"COPY_CODE","text":"Copy"}]}]`)
	payload, err := BuildTemplatePayload(components, VariableMappingInput{
		Buttons: map[string]ButtonMappingInput{
			"0": {CopyCode: "SAVE20"},
		},
	}, func(_, _ string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if payload.Components[0].Parameters[0].CouponCode != "SAVE20" {
		t.Fatalf("unexpected coupon code: %s", payload.Components[0].Parameters[0].CouponCode)
	}
}

func TestBuildComplexTemplate(t *testing.T) {
	components := json.RawMessage(`[
		{"type":"HEADER","format":"IMAGE"},
		{"type":"BODY","text":"Hi {{1}}, code {{2}}"},
		{"type":"FOOTER","text":"Terms apply"},
		{"type":"BUTTONS","buttons":[
			{"type":"URL","text":"Open","url":"https://shop.example/{{1}}"},
			{"type":"COPY_CODE","text":"Copy"}
		]}
	]`)
	payload, err := BuildTemplatePayload(components, VariableMappingInput{
		HeaderMedia: MediaReference{Link: "https://cdn.example.com/banner.png"},
		Body: map[string]string{
			"1": "name",
			"2": "code",
		},
		Buttons: map[string]ButtonMappingInput{
			"0": {URLVars: map[string]string{"1": "slug"}},
			"1": {CopyCode: "PROMO99"},
		},
	}, func(index, source string) string {
		switch index + ":" + source {
		case "1:name":
			return "Maya"
		case "2:code":
			return "ABC"
		case "1:slug":
			return "maya"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Components) != 4 {
		t.Fatalf("expected 4 send components, got %d", len(payload.Components))
	}
}

func TestBuildMissingVariableFails(t *testing.T) {
	components := json.RawMessage(`[{"type":"BODY","text":"Hello {{1}}"}]`)
	_, err := BuildTemplatePayload(components, VariableMappingInput{}, func(_, _ string) string { return "" })
	if err == nil {
		t.Fatal("expected missing variable error")
	}
}
