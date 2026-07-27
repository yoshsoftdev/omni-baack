package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/omnichannel/omni-chan-v2/internal/broadcast/worker"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/campaign/delivery"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	mediamanager "github.com/omnichannel/omni-chan-v2/internal/media"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/internal/storageclient"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/internal/websocket/hub"
	"github.com/omnichannel/omni-chan-v2/pkg/broadcastcancel"
	"github.com/omnichannel/omni-chan-v2/pkg/cryptoutil"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/logger"
	"github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/migrate"
	redisclient "github.com/omnichannel/omni-chan-v2/pkg/redis"
	"github.com/omnichannel/omni-chan-v2/pkg/workerregistry"
	"github.com/redis/go-redis/v9"
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
	slogLog := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

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

	tokenCipher, err := cryptoutil.NewTokenCipherFromBase64(cfg.Meta.TokenEncryptionKey)
	if err != nil {
		log.Error("token cipher init failed", "error", err)
		os.Exit(1)
	}

	workerCfg := worker.DefaultConfig()
	workerCfg.WorkerID = worker.ResolveWorkerID(os.Getenv("BROADCAST_WORKER_ID"))
	if v := os.Getenv("BROADCAST_SEND_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			workerCfg.RateLimitPerSec = n
		}
	}
	workerCfg = workerCfg.Normalize()

	campaignRepo := campaign.NewRepository(db)
	templateRepo := template.NewRepository(db)
	channelRepo := channel.NewRepository(db)
	metaClient := meta.NewClient(cfg.Meta.APIVersion)
	metrics := worker.NewMetrics()
	metrics.IncWorkerRestart()
	sender := worker.NewSender(metaClient, channelRepo, tokenCipher, metrics, slogLog)
	if strings.TrimSpace(cfg.StorageService.URL) != "" {
		storageClient := storageclient.New(cfg.StorageService.URL, cfg.NATS.InternalToken)
		mediaMetrics := &mediamanager.MemoryMetrics{}
		mediaMgr, err := mediamanager.NewWhatsAppMediaManagerFromMeta(
			mediamanager.NewPostgresAssetRepository(db),
			mediamanager.NewPostgresProviderMediaCacheRepository(db),
			mediamanager.NewStorageClientObjectStore(storageClient),
			metaClient,
			mediamanager.WithMediaMetrics(mediaMetrics),
			mediamanager.WithMediaLogger(slogLog),
		)
		if err != nil {
			log.Error("media manager init failed", "error", err)
			os.Exit(1)
		}
		sender = sender.WithMediaManager(mediaMgr, mediaMetrics)
	} else {
		log.Warn("storage service URL empty; template header EnsureMedia disabled")
	}

	localLimiter := worker.NewMemoryRateLimiter(workerCfg.RateLimitPerSec)
	var redisCLI *redisclient.Client
	var cancelChecker worker.CampaignCancelChecker
	var inboxRealtime inbox.RealtimePublisher
	limiter := worker.RateLimiter(localLimiter)
	if rdb, err := redisclient.New(ctx, cfg.Redis); err == nil {
		redisCLI = rdb
		defer func() { _ = rdb.Close() }()
		redisLimiter := worker.NewRedisTokenBucketLimiter(rdb.Client, workerCfg.RateLimitPerSec)
		limiter = worker.NewFallbackRateLimiter(redisLimiter, localLimiter, metrics)
		cancelChecker = broadcastcancel.NewStore(rdb.Client, broadcastcancel.DefaultTTL)
		// Fan out inbox events to API WebSocket hubs via Redis pub/sub.
		inboxRealtime = inbox.NewHubPublisher(hub.New(log, rdb.Client))
	} else {
		log.Warn("redis unavailable, using in-process rate limiter", "error", err)
	}

	contactRepo := contact.NewRepository(db)
	conversationRepo := conversation.NewRepository(db)
	teamRepo := team.NewRepository(db)
	conversationService := conversation.NewService(conversationRepo, contactRepo, teamRepo, nil)
	messageRepo := message.NewRepository(db)
	messageService := message.NewService(messageRepo, message.NewAttachmentRepository(db), conversationRepo, teamRepo, nil, nil, nil)
	inboxSettingsRepo := inbox.NewSettingsRepository(db)
	inboxSync := inbox.NewSyncService(conversationRepo, contactRepo, inboxSettingsRepo, nil, inboxRealtime)
	inboxRecorder := worker.NewCampaignInboxRecorder(conversationService, messageService, inboxSync)

	processor := worker.NewBatchProcessor(
		workerCfg,
		campaignRepo,
		campaignRepo,
		templateRepo,
		channelRepo,
		sender,
		campaignRepo,
		cancelChecker,
		limiter,
		metrics,
		slogLog,
	).WithLifecycle(delivery.NewCompletionMonitor(campaignRepo, &delivery.Metrics{}, slogLog)).
		WithInboxRecorder(inboxRecorder)

	natsCfg := sharedconfig.NATS{
		URL:              cfg.NATS.URL,
		StreamName:       cfg.NATS.StreamName,
		InternalToken:    cfg.NATS.InternalToken,
		MaxPublishRetry:  3,
		PublishRetryWait: 200 * time.Millisecond,
	}
	nc, err := sharednats.Connect(natsCfg.URL)
	if err != nil {
		log.Error("nats connect failed", "error", err)
		os.Exit(1)
	}
	defer nc.Close()

	healthAddr := os.Getenv("BROADCAST_WORKER_HTTP_ADDR")
	if healthAddr == "" {
		healthAddr = ":8091"
	}
	var redisPing *redis.Client
	if redisCLI != nil {
		redisPing = redisCLI.Client
	}
	_ = worker.ServeHealth(ctx, healthAddr, worker.NewHealthChecker(db, redisPing, nc))

	recipientRecovery := worker.NewRecipientRecovery(campaignRepo, metrics, worker.DefaultRecipientStaleAfter, slogLog)
	go recipientRecovery.Start(ctx, worker.DefaultRecipientRecoveryInterval)

	if redisCLI != nil {
		reg := workerregistry.New(redisCLI.Client, workerregistry.RoleSendWorker, workerCfg.WorkerID)
		go reg.StartHeartbeat(ctx)
	}

	consumer := worker.NewConsumer(processor, workerCfg.BroadcastStream, "broadcast-send-worker", slogLog)
	w := worker.NewWorker(consumer, nc, slogLog)
	if err := w.Start(ctx); err != nil {
		log.Error("broadcast worker failed", "error", err)
		os.Exit(1)
	}
}
