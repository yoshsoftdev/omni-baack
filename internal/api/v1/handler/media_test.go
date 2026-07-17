package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/storageclient"
	"github.com/omnichannel/omni-chan-v2/shared/mediatoken"
)

type mediaDownloaderStub struct {
	objectID uuid.UUID
	teamID   uuid.UUID
}

func (s mediaDownloaderStub) Download(
	_ context.Context,
	teamID, objectID uuid.UUID,
) (io.ReadCloser, *storageclient.StoredObject, error) {
	if teamID != s.teamID || objectID != s.objectID {
		return nil, nil, context.Canceled
	}
	return io.NopCloser(bytes.NewReader([]byte("image"))), &storageclient.StoredObject{
		ID:        objectID,
		TeamID:    teamID,
		MimeType:  "image/jpeg",
		SizeBytes: 5,
	}, nil
}

func TestMediaProxyHandlerRequiresInternalTokenAndStreams(t *testing.T) {
	const (
		secret        = "media-secret"
		internalToken = "internal-secret"
	)
	objectID := uuid.New()
	teamID := uuid.New()
	issuer := mediatoken.NewIssuer(secret, 5*time.Minute)
	token, _, err := issuer.Mint(objectID, teamID)
	if err != nil {
		t.Fatal(err)
	}

	handler := NewMediaProxyHandler(
		issuer,
		mediaDownloaderStub{objectID: objectID, teamID: teamID},
		internalToken,
	)
	app := fiber.New()
	app.Get("/api/v1/internal/media/:token", handler.Serve)

	unauthorized, err := app.Test(
		httptest.NewRequest(http.MethodGet, "/api/v1/internal/media/"+token, nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", unauthorized.StatusCode)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/internal/media/"+token, nil)
	req.Header.Set("X-Internal-Token", internalToken)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "image" {
		t.Fatalf("body=%q", body)
	}
}
