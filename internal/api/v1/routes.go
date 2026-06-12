package v1

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/auth"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/device"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	metahandler "github.com/omnichannel/omni-chan-v2/internal/meta"
	"github.com/omnichannel/omni-chan-v2/internal/onboarding"
	"github.com/omnichannel/omni-chan-v2/internal/otp"
	"github.com/omnichannel/omni-chan-v2/internal/tag"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

type Deps struct {
	AuthHandler         *auth.Handler
	TeamHandler         *team.Handler
	DeviceHandler       *device.Handler
	MetaHandler         *metahandler.Handler
	ContactHandler      *contact.Handler
	TagHandler          *tag.Handler
	ConversationHandler *conversation.Handler
	MessageHandler      *message.Handler
	TemplateHandler     *template.Handler
	CampaignHandler     *campaign.Handler
	OnboardingHandler   *onboarding.Handler
	OTPHandler          *otp.Handler
	JWT                 *jwtmanager.Manager
}

func Register(router fiber.Router, deps Deps) {
	router.Get("/health", health)

	if deps.OTPHandler != nil {
		otp.RegisterRoutes(router, deps.OTPHandler)
	}

	if deps.JWT == nil {
		return
	}
	if deps.AuthHandler != nil {
		auth.RegisterRoutes(router, deps.AuthHandler, deps.JWT)
	}
	if deps.TeamHandler != nil {
		team.RegisterRoutes(router, deps.TeamHandler, deps.JWT)
	}
	if deps.DeviceHandler != nil {
		device.RegisterRoutes(router, deps.DeviceHandler, deps.JWT)
	}
	if deps.ContactHandler != nil {
		contact.RegisterRoutes(router, deps.ContactHandler, deps.JWT)
	}
	if deps.TagHandler != nil {
		tag.RegisterRoutes(router, deps.TagHandler, deps.JWT)
	}
	if deps.ConversationHandler != nil {
		conversation.RegisterRoutes(router, deps.ConversationHandler, deps.JWT)
	}
	if deps.MessageHandler != nil {
		message.RegisterRoutes(router, deps.MessageHandler, deps.JWT)
	}
	if deps.TemplateHandler != nil {
		template.RegisterRoutes(router, deps.TemplateHandler, deps.JWT)
	}
	if deps.CampaignHandler != nil {
		campaign.RegisterRoutes(router, deps.CampaignHandler, deps.JWT)
	}
	if deps.OnboardingHandler != nil {
		onboarding.RegisterRoutes(router, deps.OnboardingHandler, deps.JWT)
	}
	if deps.MetaHandler != nil {
		metahandler.RegisterRoutes(router, deps.MetaHandler, deps.JWT)
	}
}

func health(c *fiber.Ctx) error {
	return response.OK(c, "ok", fiber.Map{
		"version": "v1",
	})
}
