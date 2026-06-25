package orchestrator

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	sharedevents "github.com/omnichannel/omni-chan-v2/shared/events"
)

// Pipeline coordinates audience resolution and batch planning for a job.
type Pipeline struct {
	campaigns CampaignStore
	audience  *AudienceResolver
	planner   *BatchPlanner
	log       *slog.Logger
}

func NewPipeline(
	campaigns CampaignStore,
	audience *AudienceResolver,
	planner *BatchPlanner,
	log *slog.Logger,
) *Pipeline {
	if log == nil {
		log = slog.Default()
	}
	return &Pipeline{campaigns: campaigns, audience: audience, planner: planner, log: log}
}

// ProcessJob runs audience resolution and batch planning idempotently.
func (p *Pipeline) ProcessJob(ctx context.Context, jobID uuid.UUID) error {
	job, err := p.campaigns.FindJobByID(ctx, jobID)
	if err != nil {
		return err
	}
	switch job.Status {
	case campaign.JobCompleted, campaign.JobFailed, campaign.JobCancelled:
		return nil
	}

	if err := p.audience.Resolve(ctx, job); err != nil {
		return err
	}
	job, err = p.campaigns.FindJobByID(ctx, jobID)
	if err != nil {
		return err
	}
	return p.planner.Plan(ctx, job)
}

// ProcessJobCreated handles broadcast.job.created payloads.
func (p *Pipeline) ProcessJobCreated(ctx context.Context, payload sharedevents.JobCreatedPayload) error {
	jobID, err := uuid.Parse(payload.JobID)
	if err != nil {
		return err
	}
	return p.ProcessJob(ctx, jobID)
}

// ProcessPendingJobs sweeps jobs that may have missed NATS delivery.
func (p *Pipeline) ProcessPendingJobs(ctx context.Context, limit int) error {
	jobs, err := p.campaigns.ListPendingJobs(ctx, limit)
	if err != nil {
		return err
	}
	for i := range jobs {
		if err := p.ProcessJob(ctx, jobs[i].ID); err != nil {
			p.log.Error("pending job processing failed",
				"job_id", jobs[i].ID.String(),
				"campaign_id", jobs[i].CampaignID.String(),
				"error", err,
			)
		}
	}
	return nil
}

// Orchestrator runs scheduler, recovery, outbox relay, and job processing loops.
type Orchestrator struct {
	cfg       Config
	scheduler *Scheduler
	pipeline  *Pipeline
	recovery  *Recovery
	outbox    *OutboxRelay
	metrics   *Metrics
	log       *slog.Logger
}

func NewOrchestrator(
	cfg Config,
	scheduler *Scheduler,
	pipeline *Pipeline,
	recovery *Recovery,
	outbox *OutboxRelay,
	metrics *Metrics,
	log *slog.Logger,
) *Orchestrator {
	cfg = cfg.Normalize()
	if log == nil {
		log = slog.Default()
	}
	if metrics == nil {
		metrics = &Metrics{}
	}
	return &Orchestrator{
		cfg:       cfg,
		scheduler: scheduler,
		pipeline:  pipeline,
		recovery:  recovery,
		outbox:    outbox,
		metrics:   metrics,
		log:       log,
	}
}

func (o *Orchestrator) Start(ctx context.Context) {
	go o.outbox.Start(ctx, o.cfg.OutboxPollInterval)
	go o.recovery.Start(ctx, o.cfg.RecoveryInterval)

	ticker := time.NewTicker(o.cfg.SchedulerInterval)
	defer ticker.Stop()

	for {
		o.runTick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (o *Orchestrator) runTick(ctx context.Context) {
	now := time.Now().UTC()
	if err := o.scheduler.Tick(ctx, now, 50); err != nil {
		o.log.Error("scheduler tick failed", "error", err)
	}
	if err := o.pipeline.ProcessPendingJobs(ctx, 20); err != nil {
		o.log.Error("pending jobs sweep failed", "error", err)
	}
}

// HandleNATSMessage processes orchestration subjects from JetStream.
func (o *Orchestrator) HandleNATSMessage(ctx context.Context, subject string, data []byte) error {
	switch subject {
	case sharedevents.SubjectBroadcastJobCreated:
		var payload sharedevents.JobCreatedPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return err
		}
		return o.pipeline.ProcessJobCreated(ctx, payload)
	default:
		return nil
	}
}

func (o *Orchestrator) Metrics() *Metrics {
	return o.metrics
}
