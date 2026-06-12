package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/omnichannel/omni-chan-v2/internal/api"
	"github.com/omnichannel/omni-chan-v2/internal/eventconsumer"
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
	"github.com/omnichannel/omni-chan-v2/internal/storage"
	"github.com/omnichannel/omni-chan-v2/internal/tag"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	wsserver "github.com/omnichannel/omni-chan-v2/internal/websocket"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/user"
	"github.com/omnichannel/omni-chan-v2/internal/websocket/hub"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/campaignqueue"
	"github.com/omnichannel/omni-chan-v2/pkg/logger"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/migrate"
	redisclient "github.com/omnichannel/omni-chan-v2/pkg/redis"
	sharedconfig "github.com/omnichannel/omni-chan-v2/shared/config"
	sharednats "github.com/omnichannel/omni-chan-v2/shared/nats"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	log := logger.New(cfg.App.Env)

	if err := migrate.Up(cfg.Database.URL, "migrations"); err != nil {
		log.Error("migration failed", "error", err)
		os.Exit(1)
	}

	db, err := database.New(ctx, cfg.Database)
	if err != nil {
		log.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	rdb, err := redisclient.New(ctx, cfg.Redis)
	if err != nil {
		log.Error("redis connection failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = rdb.Close() }()

	store, err := storage.New(ctx, cfg.Storage)
	if err != nil {
		log.Error("storage init failed", "error", err)
		os.Exit(1)
	}
	_ = store

	wsHub := hub.New(log, rdb.Client)
	go func() {
		if err := wsHub.StartRedisSubscriber(ctx); err != nil && ctx.Err() == nil {
			log.Error("websocket redis subscriber stopped", "error", err)
		}
	}()

	jwt := jwtmanager.NewManager(
		cfg.JWT.AccessSecret,
		cfg.JWT.RefreshSecret,
		cfg.JWT.AccessTTL,
	)

	userRepo := user.NewRepository(db)
	teamRepo := team.NewRepository(db)
	refreshRepo := auth.NewRefreshRepository(db)
	authService := auth.NewService(userRepo, teamRepo, refreshRepo, jwt, cfg.JWT)
	authHandler := auth.NewHandler(authService)

	teamService := team.NewService(teamRepo, userRepo)
	teamHandler := team.NewHandler(teamService)

	metaClient := metaclient.NewClient(cfg.Meta.APIVersion)

	deviceRepo := device.NewRepository(db)
	deviceService := device.NewService(deviceRepo, teamRepo, metaClient, cfg.Meta)
	deviceHandler := device.NewHandler(deviceService)

	tagRepo := tag.NewRepository(db)
	tagService := tag.NewService(tagRepo, teamRepo)
	tagHandler := tag.NewHandler(tagService)

	contactRepo := contact.NewRepository(db)
	contactService := contact.NewService(contactRepo, tagRepo, teamRepo)
	contactHandler := contact.NewHandler(contactService)

	conversationRepo := conversation.NewRepository(db)
	conversationService := conversation.NewService(conversationRepo, teamRepo, wsHub)
	conversationHandler := conversation.NewHandler(conversationService)

	messageRepo := message.NewRepository(db)
	messageService := message.NewService(messageRepo, conversationService, contactService, deviceRepo, teamRepo, userRepo, metaClient, wsHub)
	messageHandler := message.NewHandler(messageService)
	onboardingService := onboarding.NewService(deviceRepo, contactService, conversationService, messageRepo, teamRepo, metaClient)
	onboardingHandler := onboarding.NewHandler(onboardingService)
	metaHandler := metahandler.NewHandler(cfg.Meta)

	templateRepo := template.NewRepository(db)
	templateService := template.NewService(templateRepo, deviceRepo, teamRepo, metaClient)
	templateHandler := template.NewHandler(templateService)

	campaignQueue := campaignqueue.NewRedisQueue(rdb.Client)
	campaignRepo := campaign.NewRepository(db)
	campaignService := campaign.NewService(campaignRepo, contactRepo, deviceRepo, templateRepo, teamRepo, campaignQueue)
	campaignHandler := campaign.NewHandler(campaignService)
	campaignWorker := campaign.NewWorker(campaignRepo, contactRepo, deviceRepo, templateRepo, campaignQueue, metaClient, messageService, log)
	campaignScheduler := campaign.NewScheduler(campaignService, log)

	otpRepo := otp.NewRepository(db)
	otpService := otp.NewService(otpRepo, deviceRepo, metaClient, cfg.OTP.DefaultDeviceSlug)
	otpHandler := otp.NewHandler(otpService, cfg.Meta.VerifyToken)

	go campaignWorker.Start(ctx)
	go campaignScheduler.Start(ctx)

	if cfg.NATS.Enabled {
		startEventConsumer(ctx, cfg, messageService, onboardingService, log)
	}

	app := api.NewRouter(api.Deps{
		Config:              cfg,
		Logger:              log,
		DB:                  db,
		AuthHandler:         authHandler,
		TeamHandler:         teamHandler,
		DeviceHandler:       deviceHandler,
		MetaHandler:         metaHandler,
		ContactHandler:      contactHandler,
		TagHandler:          tagHandler,
		ConversationHandler: conversationHandler,
		MessageHandler:      messageHandler,
		TemplateHandler:     templateHandler,
		CampaignHandler:     campaignHandler,
		OnboardingHandler:   onboardingHandler,
		OTPHandler:          otpHandler,
		JWT:                 jwt,
	})

	wsserver.Register(app, wsHub, jwt, log)

	go func() {
		log.Info("server starting", "addr", cfg.HTTP.Addr())
		if err := app.Listen(cfg.HTTP.Addr()); err != nil {
			log.Error("server stopped", "error", err)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = shutdownCtx
	_ = app.Shutdown()
}

func startEventConsumer(
	ctx context.Context,
	cfg *config.Config,
	messageService *message.Service,
	onboardingService *onboarding.Service,
	log *logger.Logger,
) {
	slogLog := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	natsCfg := sharedconfig.NATS{
		URL:           cfg.NATS.URL,
		StreamName:    cfg.NATS.StreamName,
		InternalToken: cfg.NATS.InternalToken,
	}

	nc, err := sharednats.Connect(natsCfg.URL)
	if err != nil {
		log.Error("nats connect failed", "error", err)
		return
	}

	sub, err := sharednats.NewSubscriber(nc, natsCfg, slogLog)
	if err != nil {
		log.Error("nats subscriber init failed", "error", err)
		nc.Close()
		return
	}

	consumer := eventconsumer.New(messageService, onboardingService, slogLog)
	if err := sub.Subscribe(ctx, cfg.NATS.ConsumerName, consumer.Handle); err != nil {
		log.Error("nats subscribe failed", "error", err)
		nc.Close()
		return
	}

	go func() {
		<-ctx.Done()
		nc.Close()
	}()

	log.Info("webhook event consumer started", "consumer", cfg.NATS.ConsumerName)
}
