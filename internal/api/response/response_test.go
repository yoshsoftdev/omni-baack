package response

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestOK(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		return OK(c, "ok", fiber.Map{"status": "healthy"})
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test() error = %v", err)
	}
	defer resp.Body.Close()

	body, err := decodeSuccessBody(resp.Body)
	if err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !body.Success || body.Message != "ok" {
		t.Errorf("body = %+v", body)
	}
}

func TestBadRequest(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		return BadRequest(c, "invalid")
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d", resp.StatusCode)
	}

	body, err := decodeFailBody(resp.Body)
	if err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Success || body.Error.Code != CodeValidationError || body.Error.Message != "invalid" {
		t.Errorf("body = %+v", body)
	}
}

func TestFeatureNotAvailable(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		return FeatureNotAvailable(c, "not ready")
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test() error = %v", err)
	}
	defer resp.Body.Close()

	body, err := decodeFailBody(resp.Body)
	if err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != CodeFeatureNotAvailable {
		t.Errorf("code = %s", body.Error.Code)
	}
}

func decodeSuccessBody(r io.Reader) (Body, error) {
	var body Body
	err := json.NewDecoder(r).Decode(&body)
	return body, err
}

func decodeFailBody(r io.Reader) (FailBody, error) {
	var body FailBody
	err := json.NewDecoder(r).Decode(&body)
	return body, err
}
