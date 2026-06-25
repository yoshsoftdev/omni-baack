package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/omnichannel/omni-chan-v2/internal/broadcast/orchestrator"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/segment"
	"github.com/omnichannel/omni-chan-v2/internal/tag"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/logger"
	"github.com/omnichannel/omni-chan-v2/pkg/migrate"
	redisclient "github.com/omnichannel/omni-chan-v2/pkg/redis"
	sharedconfig "github.com/omnichannel/omni-chan-v2/shared/config"
	sharednats "github.com/omnichannel/omni-chan-v2/shared/nats"
	"github.com/omnichannel/omni-chan-v2/pkg/workerregistry"
	sharedevents "github.com/omnichannel/omni-chan-v2/shared/events"
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

	orchCfg := orchestrator.DefaultConfig()
	if v := os.Getenv("BROADCAST_BATCH_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			orchCfg.BatchSize = n
		}
	}
	orchCfg = orchCfg.Normalize()

	campaignRepo := campaign.NewRepository(db)
	contactRepo := contact.NewRepository(db)
	segmentRepo := segment.NewRepository(db)
	tagRepo := tag.NewRepository(db)
	metrics := &orchestrator.Metrics{}

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

	publisher, err := sharednats.NewBroadcastPublisher(nc, natsCfg, orchCfg.BroadcastStream, slogLog)
	if err != nil {
		log.Error("broadcast publisher init failed", "error", err)
		os.Exit(1)
	}

	outboxRelay := orchestrator.NewOutboxRelay(campaignRepo, publisher, slogLog)
	scheduler := orchestrator.NewScheduler(campaignRepo, campaignRepo, metrics, slogLog)
	audience := orchestrator.NewAudienceResolver(campaignRepo, contactRepo, segmentRepo, tagRepo, metrics, slogLog, orchCfg.AudienceChunkSize)
	planner := orchestrator.NewBatchPlanner(campaignRepo, publisher, campaignRepo, metrics, orchCfg.BatchSize, slogLog)
	pipeline := orchestrator.NewPipeline(campaignRepo, audience, planner, slogLog)
	recovery := orchestrator.NewRecovery(campaignRepo, publisher, campaignRepo, metrics, orchCfg.BatchLease, slogLog)
	orch := orchestrator.NewOrchestrator(orchCfg, scheduler, pipeline, recovery, outboxRelay, metrics, slogLog)

	orchID := os.Getenv("BROADCAST_ORCHESTRATOR_ID")
	if orchID == "" {
		orchID = "orchestrator-1"
	}
	if rdb, err := redisclient.New(ctx, cfg.Redis); err == nil {
		defer func() { _ = rdb.Close() }()
		reg := workerregistry.New(rdb.Client, workerregistry.RoleOrchestrator, orchID)
		go reg.StartHeartbeat(ctx)
	} else {
		log.Warn("redis unavailable, orchestrator heartbeat disabled", "error", err)
	}

	go subscribeJobCreated(ctx, nc, orchCfg.BroadcastStream, orch, slogLog)
	go orch.Start(ctx)

	log.Info("broadcast orchestrator started")
	<-ctx.Done()
	log.Info("broadcast orchestrator shutting down")
}

func subscribeJobCreated(ctx context.Context, nc *nats.Conn, stream string, orch *orchestrator.Orchestrator, log *slog.Logger) {
	js, err := nc.JetStream()
	if err != nil {
		log.Error("jetstream unavailable", "error", err)
		return
	}
	_, err = js.AddConsumer(stream, &nats.ConsumerConfig{
		Durable:       "broadcast-orchestrator",
		FilterSubject: sharedevents.SubjectBroadcastJobCreated,
		AckPolicy:     nats.AckExplicitPolicy,
	})
	if err != nil && err != nats.ErrConsumerNameAlreadyInUse {
		log.Warn("add consumer", "error", err)
	}
	_, err = js.Subscribe(sharedevents.SubjectBroadcastJobCreated, func(msg *nats.Msg) {
		if err := orch.HandleNATSMessage(ctx, msg.Subject, msg.Data); err != nil {
			log.Error("job created handler failed", "error", err)
			_ = msg.Nak()
			return
		}
		_ = msg.Ack()
	}, nats.Bind(stream, "broadcast-orchestrator"))
	if err != nil {
		log.Error("subscribe job.created failed", "error", err)
	}
}
