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
	eventconsumeridempotency "github.com/omnichannel/omni-chan-v2/internal/eventconsumer/idempotency"
	"github.com/omnichannel/omni-chan-v2/internal/auth"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/campaign/delivery"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/dashboard"
	"github.com/omnichannel/omni-chan-v2/internal/device"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	metasvc "github.com/omnichannel/omni-chan-v2/internal/meta"
	"github.com/omnichannel/omni-chan-v2/internal/onboarding"
	"github.com/omnichannel/omni-chan-v2/internal/operations"
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
	"github.com/omnichannel/omni-chan-v2/pkg/broadcastcancel"
	"github.com/omnichannel/omni-chan-v2/pkg/cryptoutil"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/logger"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/migrate"
	"github.com/omnichannel/omni-chan-v2/pkg/uow"
	redisclient "github.com/omnichannel/omni-chan-v2/pkg/redis"
	"github.com/omnichannel/omni-chan-v2/pkg/workerregistry"
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

	metaClient := metaclient.NewClient(cfg.Meta.APIVersion)
	metaAPI := metasvc.NewGraphClient(metaClient)
	tokenCipher, err := cryptoutil.NewTokenCipherFromBase64(cfg.Meta.TokenEncryptionKey)
	if err != nil {
		log.Error("meta token cipher init failed", "error", err)
		os.Exit(1)
	}
	syncRunRepo := template.NewInMemorySyncRunRepository()
	metaSyncService := template.NewSyncService(metaClient, channelRepo, templateRepo, syncRunRepo, tokenCipher, log.Logger)
	metaHealthService := metasvc.NewHealthService(metaAPI, channelRepo, tokenCipher, metaSyncService)
	embeddedSignupSessionRepo := metasvc.NewEmbeddedSignupSessionRepository(db)

	workspaceService := workspace.NewService(
		teamRepo, quotaSvc, rbacService,
		channelRepo, templateRepo, contactRepo, campaignRepo,
		workspace.NewSnapshotRepository(db),
		metaSyncService,
	)
	embeddedSignupService := metasvc.NewEmbeddedSignupService(
		metaAPI, channelRepo, embeddedSignupSessionRepo, tokenCipher, cfg.Meta, workspaceService, log.Logger,
	)
	channelService := channel.NewService(channelRepo, teamRepo, quotaSvc, rbacService, tokenCipher, metaAPI)
	teamService := team.NewService(teamRepo, userRepo, quotaSvc, rbacService)
	segmentService := segment.NewService(segmentRepo, teamRepo, rbacService)

	deviceRepo := device.NewRepository(db)

	tagRepo := tag.NewRepository(db)
	tagService := tag.NewService(tagRepo, teamRepo, rbacService)

	contactService := contact.NewService(contactRepo, teamRepo, quotaSvc, rbacService)

	conversationRepo := conversation.NewRepository(db)
	conversationService := conversation.NewService(conversationRepo, contactRepo, teamRepo, rbacService)

	inboxSettingsRepo := inbox.NewSettingsRepository(db)
	inboxAssignment := inbox.NewAssignmentEngine(teamRepo, inboxSettingsRepo, conversationRepo)
	inboxRealtime := inbox.NewHubPublisher(wsHub)
	inboxSync := inbox.NewSyncService(conversationRepo, inboxSettingsRepo, inboxAssignment, inboxRealtime)
	presenceService := inbox.NewPresenceService(inbox.NewPresenceRepository(db))
	conversationService.SetInboxSync(inboxSync)

	messageRepo := message.NewRepository(db)
	messageService := message.NewService(messageRepo, conversationRepo, teamRepo, rbacService)
	onboardingService := onboarding.NewService(deviceRepo, contactRepo, conversationRepo, messageRepo, teamRepo, metaClient)

	templateService := template.NewService(templateRepo, channelRepo, teamRepo, rbacService, metaSyncService)

	campaignTx := uow.NewCampaignRunner(db)
	cancelFlags := broadcastcancel.NewStore(rdb.Client, broadcastcancel.DefaultTTL)
	campaignService := campaign.NewService(
		campaignRepo, campaignTx, teamRepo, channelRepo, templateRepo, segmentRepo, rbacService,
		campaign.WithCancelFlags(cancelFlags),
	)

	operationsRepo := operations.NewRepository(db)
	operationsService := operations.NewService(operationsRepo, operations.NewRedisWorkerCounter(rdb.Client), teamRepo, rbacService)

	reportRepo := report.NewRepository(db)
	reportService := report.NewService(reportRepo, teamRepo, rbacService)

	dashboardRepo := dashboard.NewRepository(db)
	dashboardService := dashboard.NewService(dashboardRepo, teamRepo, workspaceService, rbacService)

	deliveryMetrics := &delivery.Metrics{}
	deliveryReconciler := delivery.NewReconciler(campaignRepo, deliveryMetrics, log.Logger, 30*24*time.Hour)
	go deliveryReconciler.StartDaily(ctx)

	if cfg.NATS.Enabled {
		startEventConsumer(ctx, cfg, channelRepo, contactService, conversationService, messageService, templateRepo, onboardingService, inboxSync, campaignRepo, reportRepo, rdb, log)
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
			Channel:      handler.NewChannelHandler(channelService, metaHealthService, embeddedSignupService),
			Template:     handler.NewTemplateHandler(templateService),
			Contact:      handler.NewContactHandler(contactService),
			Segment:      handler.NewSegmentHandler(segmentService),
			Tag:          handler.NewTagHandler(tagService),
			Campaign:     handler.NewCampaignHandler(campaignService),
			Operations:   handler.NewOperationsHandler(operationsService),
			Report:       handler.NewReportHandler(reportService),
			Conversation: handler.NewConversationHandler(conversationService),
			Message:      handler.NewMessageHandler(messageService, conversationService, inboxSync),
			Dashboard:    handler.NewDashboardHandler(dashboardService),
			Webhook:      handler.NewWebhookHandler(cfg.Meta.VerifyToken),
			Meta:         metasvc.NewHandler(cfg.Meta),
			ActorResolver: actorResolver,
			JWT:           jwt,
		},
	})

	wsserver.Register(app, wsHub, jwt, log, presenceService)

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
	channelRepo channel.Repository,
	contactService *contact.Service,
	conversationService *conversation.Service,
	messageService *message.Service,
	templateRepo template.Repository,
	onboardingService *onboarding.Service,
	inboxSync *inbox.SyncService,
	campaignRepo campaign.Repository,
	reportRepo report.Repository,
	rdb *redisclient.Client,
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

	var idempStore eventconsumeridempotency.Store
	if pingErr := rdb.Client.Ping(ctx).Err(); pingErr != nil {
		log.Warn("redis idempotency unavailable, using memory store", "error", pingErr)
		idempStore = eventconsumeridempotency.NewMemoryStore(24 * time.Hour)
	} else {
		idempStore = eventconsumeridempotency.NewRedisStore(rdb.Client, 24*time.Hour)
	}

	metaSub, err := sharednats.NewMetaSubscriber(nc, natsCfg, slogLog)
	if err != nil {
		log.Error("meta subscriber init failed", "error", err)
		nc.Close()
		return
	}

	legacySub, err := sharednats.NewSubscriber(nc, natsCfg, slogLog)
	if err != nil {
		log.Error("legacy subscriber init failed", "error", err)
		nc.Close()
		return
	}

	deliveryMetrics := &delivery.Metrics{}
	deliveryAggregator := delivery.NewReportAggregator(campaignRepo, reportRepo)
	deliveryCompletion := delivery.NewCompletionMonitor(campaignRepo, deliveryMetrics, slogLog)
	deliveryTracker := delivery.NewTracker(campaignRepo, deliveryAggregator, deliveryCompletion, deliveryMetrics, slogLog)

	registry := eventconsumer.NewRegistry(eventconsumer.Deps{
		Channels:      channelRepo,
		Contacts:      contactService,
		Conversations: conversationService,
		Messages:      messageService,
		Templates:     templateRepo,
		Onboarding:    onboardingService,
		Inbox:         inboxSync,
		Delivery:      deliveryTracker,
		Idempotency:   idempStore,
		DLQ:           metaSub,
		Logger:        slogLog,
	})

	if err := metaSub.Subscribe(ctx, cfg.NATS.ConsumerName+"-meta", registry.HandleCanonical); err != nil {
		log.Error("meta subscribe failed", "error", err)
		nc.Close()
		return
	}
	if err := legacySub.Subscribe(ctx, cfg.NATS.ConsumerName+"-legacy", registry.HandleLegacy); err != nil {
		log.Error("legacy subscribe failed", "error", err)
		nc.Close()
		return
	}

	go func() {
		<-ctx.Done()
		nc.Close()
	}()

	consumerID := cfg.NATS.ConsumerName
	if consumerID == "" {
		consumerID = "event-consumer"
	}
	reg := workerregistry.New(rdb.Client, workerregistry.RoleEventConsumer, consumerID)
	go reg.StartHeartbeat(ctx)

	log.Info("event consumers started",
		"meta_consumer", cfg.NATS.ConsumerName+"-meta",
		"legacy_consumer", cfg.NATS.ConsumerName+"-legacy",
	)
}
