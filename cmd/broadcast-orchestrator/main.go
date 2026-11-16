package main

import (
	"context"
	"errors"
	"github.com/omnichannel/omni-chan-v2/shared/observability"
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
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/logger"
	"github.com/omnichannel/omni-chan-v2/pkg/migrate"
	redisclient "github.com/omnichannel/omni-chan-v2/pkg/redis"
	"github.com/omnichannel/omni-chan-v2/pkg/workerregistry"
	sharedconfig "github.com/omnichannel/omni-chan-v2/shared/config"
	sharedevents "github.com/omnichannel/omni-chan-v2/shared/events"
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
	slogLog := slog.New(observability.WrapLogHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	shutdownTracing := observability.SetupTracing(ctx, "broadcast-orchestrator", slogLog)
	defer func() { _ = shutdownTracing(context.Background()) }()

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
	metrics := &orchestrator.Metrics{}

	// LoadNATS reads the same NATS_* env as the webhook service, including stream retention and replicas.
	natsCfg := sharedconfig.LoadNATS()

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
	audience := orchestrator.NewAudienceResolver(campaignRepo, metrics, slogLog)
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

	obsReg := observability.NewRegistry()
	obsReg.MustRegister(observability.NewSnapshotCollector("omni_orchestrator", metrics.Snapshot))
	if js, jsErr := nc.JetStream(); jsErr == nil {
		obsReg.MustRegister(sharednats.NewJetStreamCollector(js, orchCfg.BroadcastStream, "broadcast-orchestrator"))
	}
	observability.Serve(ctx, observability.MetricsAddr("127.0.0.1:9103"), obsReg, map[string]observability.Check{
		"postgres": func(ctx context.Context) error { return db.Pool.Ping(ctx) },
		"nats": func(context.Context) error {
			if !nc.IsConnected() {
				return errors.New("nats disconnected")
			}
			return nil
		},
	}, slogLog)
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
	// Must use PullSubscribe: AddConsumer without DeliverSubject creates a pull consumer,
	// and js.Subscribe(... Bind) fails with "must use pull subscribe to bind to pull based consumer".
	sub, err := js.PullSubscribe(
		sharedevents.SubjectBroadcastJobCreated,
		"broadcast-orchestrator",
		nats.BindStream(stream),
		nats.ManualAck(),
	)
	if err != nil {
		log.Error("subscribe job.created failed", "error", err)
		return
	}
	log.Info("broadcast job.created subscriber started", "stream", stream, "consumer", "broadcast-orchestrator")
	redelivery := sharednats.RedeliveryPolicyFrom(sharedconfig.NATS{})

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		msgs, err := sub.Fetch(10, nats.MaxWait(2*time.Second))
		if err != nil {
			if err == nats.ErrTimeout || err == context.Canceled || err == context.DeadlineExceeded {
				continue
			}
			log.Error("job.created fetch failed", "error", err)
			sharednats.PauseAfterFetchError(ctx)
			continue
		}
		for _, msg := range msgs {
			msgCtx, span := sharednats.ConsumerSpan(ctx, msg)
			err := orch.HandleNATSMessage(msgCtx, msg.Subject, msg.Data)
			span.End()
			if err != nil {
				// Backoff, never terminate: orchestrator recovery owns final failure.
				attempt := redelivery.NakWithBackoff(msg)
				log.Error("job created handler failed", "attempt", attempt, "error", err)
				continue
			}
			_ = msg.Ack()
		}
	}
}
