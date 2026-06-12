package otp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestHandler_Send_InvalidToken(t *testing.T) {
	app := fiber.New()
	handler := NewHandler(&Service{defaultDeviceSlug: "dgos-waba"}, "secret-token")
	app.Post("/otp/send", handler.Send)

	body, _ := json.Marshal(map[string]string{
		"token": "wrong",
		"phone": "+628123456789",
		"code":  "1234",
	})
	req := httptest.NewRequest("POST", "/otp/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusUnauthorized)
	}
}

func TestHandler_Send_MissingToken(t *testing.T) {
	app := fiber.New()
	handler := NewHandler(&Service{defaultDeviceSlug: "dgos-waba"}, "secret-token")
	app.Post("/otp/send", handler.Send)

	body, _ := json.Marshal(map[string]string{
		"phone": "+628123456789",
		"code":  "1234",
	})
	req := httptest.NewRequest("POST", "/otp/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusUnauthorized)
	}
}

func TestHandler_validateToken(t *testing.T) {
	handler := NewHandler(nil, "277353")

	if err := handler.validateToken("277353"); err != nil {
		t.Fatalf("expected valid token, got %v", err)
	}
	if err := handler.validateToken("bad"); err == nil {
		t.Fatal("expected invalid token error")
	}
}

func TestHandler_Send_ValidTokenPassesToService(t *testing.T) {
	app := fiber.New()
	svc := NewService(&mockOTPRepo{}, &mockDeviceRepo{
		slug: "dgos-waba",
		device: nil,
	}, &mockMetaClient{}, "dgos-waba")
	handler := NewHandler(svc, "secret-token")
	app.Post("/otp/send", handler.Send)

	body, _ := json.Marshal(map[string]string{
		"token": "secret-token",
		"phone": "0812",
		"code":  "1234",
	})
	req := httptest.NewRequest("POST", "/otp/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	// Token OK — service rejects invalid phone with 400, not 401.
	if resp.StatusCode == fiber.StatusUnauthorized {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected unauthorized: %s", string(bodyBytes))
	}
}
