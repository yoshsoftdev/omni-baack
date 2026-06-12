package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	v1 "github.com/omnichannel/omni-chan-v2/internal/api/v1"
	"github.com/omnichannel/omni-chan-v2/internal/auth"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/config"
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
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/logger"
)

type Deps struct {
	Config              *config.Config
	Logger              *logger.Logger
	DB                  *database.DB
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

func NewRouter(deps Deps) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      deps.Config.App.Name,
		ReadTimeout:  deps.Config.HTTP.ReadTimeout,
		WriteTimeout: deps.Config.HTTP.WriteTimeout,
	})

	app.Use(recover.New())
	app.Use(requestid.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization",
	}))

	apiGroup := app.Group("/api")
	v1.Register(apiGroup.Group("/v1"), v1.Deps{
		AuthHandler:         deps.AuthHandler,
		TeamHandler:         deps.TeamHandler,
		DeviceHandler:       deps.DeviceHandler,
		MetaHandler:         deps.MetaHandler,
		ContactHandler:      deps.ContactHandler,
		TagHandler:          deps.TagHandler,
		ConversationHandler: deps.ConversationHandler,
		MessageHandler:      deps.MessageHandler,
		TemplateHandler:     deps.TemplateHandler,
		CampaignHandler:     deps.CampaignHandler,
		OnboardingHandler:   deps.OnboardingHandler,
		OTPHandler:          deps.OTPHandler,
		JWT:                 deps.JWT,
	})

	return app
}
