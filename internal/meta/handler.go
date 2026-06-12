package meta

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/config"
)

type Handler struct {
	cfg config.MetaConfig
}

func NewHandler(cfg config.MetaConfig) *Handler {
	return &Handler{cfg: cfg}
}

type EmbeddedConfigResponse struct {
	AppID      string `json:"app_id"`
	ConfigID   string `json:"config_id"`
	APIVersion string `json:"api_version"`
	Enabled    bool   `json:"enabled"`
}

// EmbeddedConfig returns public Meta Embedded Signup configuration for the frontend.
func (h *Handler) EmbeddedConfig(c *fiber.Ctx) error {
	enabled := h.cfg.AppID != "" && h.cfg.ConfigID != ""
	return response.OK(c, "ok", EmbeddedConfigResponse{
		AppID:      h.cfg.AppID,
		ConfigID:   h.cfg.ConfigID,
		APIVersion: h.cfg.APIVersion,
		Enabled:    enabled,
	})
}
