package orchestrator

import (
	"time"
)

const (
	DefaultBatchSize          = 100
	DefaultSchedulerInterval  = 30 * time.Second
	DefaultRecoveryInterval   = 5 * time.Minute
	DefaultBatchLease         = 10 * time.Minute
	DefaultOutboxPollInterval = 5 * time.Second
)

// Config tunes broadcast orchestration behavior.
type Config struct {
	BatchSize          int
	SchedulerInterval  time.Duration
	RecoveryInterval   time.Duration
	BatchLease         time.Duration
	OutboxPollInterval time.Duration
	BroadcastStream    string
	AudienceChunkSize  int
}

func DefaultConfig() Config {
	return Config{
		BatchSize:          DefaultBatchSize,
		SchedulerInterval:  DefaultSchedulerInterval,
		RecoveryInterval:   DefaultRecoveryInterval,
		BatchLease:         DefaultBatchLease,
		OutboxPollInterval: DefaultOutboxPollInterval,
		BroadcastStream:    "BROADCAST",
		AudienceChunkSize:  5000,
	}
}

func (c Config) Normalize() Config {
	if c.BatchSize <= 0 {
		c.BatchSize = DefaultBatchSize
	}
	if c.SchedulerInterval <= 0 {
		c.SchedulerInterval = DefaultSchedulerInterval
	}
	if c.RecoveryInterval <= 0 {
		c.RecoveryInterval = DefaultRecoveryInterval
	}
	if c.BatchLease <= 0 {
		c.BatchLease = DefaultBatchLease
	}
	if c.OutboxPollInterval <= 0 {
		c.OutboxPollInterval = DefaultOutboxPollInterval
	}
	if c.AudienceChunkSize <= 0 {
		c.AudienceChunkSize = 5000
	}
	if c.BroadcastStream == "" {
		c.BroadcastStream = "BROADCAST"
	}
	return c
}
