package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	v1 "github.com/omnichannel/omni-chan-v2/internal/api/v1"
	"github.com/omnichannel/omni-chan-v2/internal/auth"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/internal/device"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	metahandler "github.com/omnichannel/omni-chan-v2/internal/meta"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/logger"
)

type Deps struct {
	Config      *config.Config
	Logger      *logger.Logger
	DB          *database.DB
	AuthHandler *auth.Handler
	TeamHandler   *team.Handler
	DeviceHandler *device.Handler
	InboxHandler  *inbox.Handler
	MetaHandler   *metahandler.Handler
	JWT           *jwtmanager.Manager
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
		AuthHandler:   deps.AuthHandler,
		TeamHandler:   deps.TeamHandler,
		DeviceHandler: deps.DeviceHandler,
		InboxHandler:  deps.InboxHandler,
		MetaHandler:   deps.MetaHandler,
		JWT:           deps.JWT,
	})

	return app
}
