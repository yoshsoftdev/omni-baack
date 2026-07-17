package handler

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/storageclient"
	"github.com/omnichannel/omni-chan-v2/shared/mediatoken"
)

type mediaDownloader interface {
	Download(ctx context.Context, teamID, objectID uuid.UUID) (io.ReadCloser, *storageclient.StoredObject, error)
}

// MediaProxyHandler streams media to the public webhook-service over the
// internal network. It is never exposed directly to the internet.
type MediaProxyHandler struct {
	tokens        *mediatoken.Issuer
	storage       mediaDownloader
	internalToken string
}

func NewMediaProxyHandler(tokens *mediatoken.Issuer, storage mediaDownloader, internalToken string) *MediaProxyHandler {
	return &MediaProxyHandler{
		tokens:        tokens,
		storage:       storage,
		internalToken: strings.TrimSpace(internalToken),
	}
}

func (h *MediaProxyHandler) Serve(c *fiber.Ctx) error {
	if h == nil || h.tokens == nil || h.storage == nil {
		return response.InternalError(c, "media proxy is not configured")
	}
	if h.internalToken == "" || strings.TrimSpace(c.Get("X-Internal-Token")) != h.internalToken {
		return response.Unauthorized(c, "unauthorized")
	}
	token := strings.TrimSpace(c.Params("token"))
	objectID, teamID, err := h.tokens.Parse(token)
	if err != nil {
		if errors.Is(err, mediatoken.ErrExpiredToken) {
			return response.Unauthorized(c, "media token expired")
		}
		return response.Unauthorized(c, "invalid media token")
	}

	reader, meta, err := h.storage.Download(c.Context(), teamID, objectID)
	if err != nil {
		return response.NotFound(c, "media not found")
	}

	contentType := "application/octet-stream"
	if meta != nil && strings.TrimSpace(meta.MimeType) != "" {
		contentType = meta.MimeType
	}
	c.Set("Content-Type", contentType)
	c.Set("Cache-Control", "private, no-store")
	if meta != nil && meta.SizeBytes > 0 {
		return c.SendStream(reader, int(meta.SizeBytes))
	}
	return c.SendStream(reader)
}
