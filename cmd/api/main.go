package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/omnichannel/omni-chan-v2/internal/api"
	"github.com/omnichannel/omni-chan-v2/internal/api/v1/handler"
	v1 "github.com/omnichannel/omni-chan-v2/internal/api/v1"
	"github.com/omnichannel/omni-chan-v2/internal/eventconsumer"
	"github.com/omnichannel/omni-chan-v2/internal/auth"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/dashboard"
	"github.com/omnichannel/omni-chan-v2/internal/device"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/internal/onboarding"
	"github.com/omnichannel/omni-chan-v2/internal/quota"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/report"
	"github.com/omnichannel/omni-chan-v2/internal/segment"
	"github.com/omnichannel/omni-chan-v2/internal/storage"
	"github.com/omnichannel/omni-chan-v2/internal/tag"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	wsserver "github.com/omnichannel/omni-chan-v2/internal/websocket"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/user"
	"github.com/omnichannel/omni-chan-v2/internal/workspace"
	"github.com/omnichannel/omni-chan-v2/internal/websocket/hub"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
	"github.com/omnichannel/omni-chan-v2/pkg/campaignqueue"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/logger"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/migrate"
	"github.com/omnichannel/omni-chan-v2/pkg/uow"
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

	rbacRepo := rbac.NewRepository(db)
	rbacService := rbac.NewService(rbacRepo)
	if err := rbacService.WarmCache(ctx); err != nil {
		log.Error("rbac cache warm failed", "error", err)
		os.Exit(1)
	}

	userRepo := user.NewRepository(db)
	teamRepo := team.NewRepository(db)
	channelRepo := channel.NewRepository(db)
	segmentRepo := segment.NewRepository(db)
	contactRepo := contact.NewRepository(db)
	quotaSvc := quota.NewService(teamRepo, channelRepo, contactRepo)
	refreshRepo := auth.NewRefreshRepository(db)
	authService := auth.NewService(userRepo, teamRepo, refreshRepo, jwt, cfg.JWT)
	actorResolver := auth.NewResolver(teamRepo, rbacService)

	templateRepo := template.NewRepository(db)
	campaignRepo := campaign.NewRepository(db)

	workspaceService := workspace.NewService(
		teamRepo, quotaSvc, rbacService,
		channelRepo, templateRepo, contactRepo, campaignRepo,
		workspace.NewSnapshotRepository(db),
	)
	channelService := channel.NewService(channelRepo, teamRepo, quotaSvc, rbacService)
	teamService := team.NewService(teamRepo, userRepo, quotaSvc, rbacService)
	segmentService := segment.NewService(segmentRepo, teamRepo, rbacService)

	metaClient := metaclient.NewClient(cfg.Meta.APIVersion)
	deviceRepo := device.NewRepository(db)

	tagRepo := tag.NewRepository(db)
	tagService := tag.NewService(tagRepo, teamRepo, rbacService)

	contactService := contact.NewService(contactRepo, teamRepo, quotaSvc, rbacService)

	conversationRepo := conversation.NewRepository(db)
	conversationService := conversation.NewService(conversationRepo, contactRepo, teamRepo, rbacService)

	messageRepo := message.NewRepository(db)
	messageService := message.NewService(messageRepo, conversationRepo, teamRepo, rbacService)
	onboardingService := onboarding.NewService(deviceRepo, contactRepo, conversationRepo, messageRepo, teamRepo, metaClient)

	templateService := template.NewService(templateRepo, channelRepo, teamRepo, rbacService)

	campaignQueue := campaignqueue.NewRedisQueue(rdb.Client)
	campaignTx := uow.NewCampaignRunner(db)
	campaignService := campaign.NewService(campaignRepo, campaignTx, teamRepo, channelRepo, templateRepo, segmentRepo, rbacService)
	campaignWorker := campaign.NewWorker(campaignRepo, contactRepo, deviceRepo, templateRepo, campaignQueue, metaClient, messageService, log)
	campaignScheduler := campaign.NewScheduler(campaignService, log)

	reportRepo := report.NewRepository(db)
	reportService := report.NewService(reportRepo, teamRepo, rbacService)

	dashboardRepo := dashboard.NewRepository(db)
	dashboardService := dashboard.NewService(dashboardRepo, teamRepo, workspaceService, rbacService)

	go campaignWorker.Start(ctx)
	go campaignScheduler.Start(ctx)

	if cfg.NATS.Enabled {
		startEventConsumer(ctx, cfg, messageService, onboardingService, log)
	}

	app := api.NewRouter(api.Deps{
		Config: cfg,
		Logger: log,
		DB:     db,
		JWT:    jwt,
		V1: v1.Deps{
			Auth:         handler.NewAuthHandler(authService),
			Workspace:    handler.NewWorkspaceHandler(workspaceService),
			Team:         handler.NewTeamHandler(teamService, rbacService),
			Channel:      handler.NewChannelHandler(channelService),
			Template:     handler.NewTemplateHandler(templateService),
			Contact:      handler.NewContactHandler(contactService),
			Segment:      handler.NewSegmentHandler(segmentService),
			Tag:          handler.NewTagHandler(tagService),
			Campaign:     handler.NewCampaignHandler(campaignService),
			Report:       handler.NewReportHandler(reportService),
			Conversation: handler.NewConversationHandler(conversationService),
			Message:      handler.NewMessageHandler(messageService),
			Dashboard:    handler.NewDashboardHandler(dashboardService),
			ActorResolver: actorResolver,
			JWT:           jwt,
		},
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
