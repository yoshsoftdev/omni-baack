package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/api/v1/dto"
	"github.com/omnichannel/omni-chan-v2/internal/report"
)

type ReportHandler struct {
	svc *report.Service
}

func NewReportHandler(svc *report.Service) *ReportHandler {
	return &ReportHandler{svc: svc}
}

func (h *ReportHandler) Dashboard(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	from, err := parseTimeQuery(c, "from")
	if err != nil {
		return response.BadRequest(c, "from query must be RFC3339")
	}
	to, err := parseTimeQuery(c, "to")
	if err != nil {
		return response.BadRequest(c, "to query must be RFC3339")
	}
	channelID, err := optionalUUIDQuery(c, "channel_id")
	if err != nil {
		return response.BadRequest(c, "invalid channel_id")
	}
	stats, err := h.svc.GetDashboard(c.Context(), actor.TeamID, actor.UserID, actor.Role, report.DashboardInput{
		From:      from,
		To:        to,
		ChannelID: channelID,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", stats)
}

func (h *ReportHandler) DailyTrend(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	from, err := parseTimeQuery(c, "from")
	if err != nil {
		return response.BadRequest(c, "from query must be RFC3339")
	}
	to, err := parseTimeQuery(c, "to")
	if err != nil {
		return response.BadRequest(c, "to query must be RFC3339")
	}
	stats, err := h.svc.ListDailyTrend(c.Context(), actor.TeamID, actor.UserID, actor.Role, from, to)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"items": stats})
}

func (h *ReportHandler) ListCampaigns(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	from, err := parseTimeQuery(c, "from")
	if err != nil {
		return response.BadRequest(c, "from query must be RFC3339")
	}
	to, err := parseTimeQuery(c, "to")
	if err != nil {
		return response.BadRequest(c, "to query must be RFC3339")
	}
	params, err := pageParams(c)
	if err != nil {
		return mapError(c, err)
	}
	page, err := h.svc.ListCampaignReports(c.Context(), actor.TeamID, actor.UserID, actor.Role, from, to, params)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", dto.NewPageResponse(page))
}

func (h *ReportHandler) GetCampaign(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid campaign id")
	}
	funnel, err := h.svc.GetCampaignReport(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", funnel)
}

func (h *ReportHandler) ListLinkClicks(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid campaign id")
	}
	params, err := pageParams(c)
	if err != nil {
		return mapError(c, err)
	}
	page, err := h.svc.ListLinkClicks(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, params)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", dto.NewPageResponse(page))
}
