package template

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// SyncRunRepository persists template sync execution records.
type SyncRunRepository interface {
	Create(ctx context.Context, run *SyncRun) error
	Update(ctx context.Context, run *SyncRun) error
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*SyncRun, error)
	List(ctx context.Context, teamID uuid.UUID, filter SyncRunListFilter, page repository.PageParams) (repository.Page[SyncRun], error)
	LastSuccessfulByChannel(ctx context.Context, channelID uuid.UUID) (*SyncRun, error)
	CountSuccessfulWithin(ctx context.Context, teamID uuid.UUID, since time.Time) (int64, error)
	MetricsByChannel(ctx context.Context, teamID, channelID uuid.UUID) (*SyncMetrics, error)
	MetricsByTeam(ctx context.Context, teamID uuid.UUID) (*SyncMetrics, error)
}

type SyncRunListFilter struct {
	ChannelID *uuid.UUID
}

// InMemorySyncRunRepository stores sync runs in process memory (until DB migration lands).
type InMemorySyncRunRepository struct {
	mu   sync.RWMutex
	runs map[uuid.UUID]*SyncRun
}

func NewInMemorySyncRunRepository() *InMemorySyncRunRepository {
	return &InMemorySyncRunRepository{runs: make(map[uuid.UUID]*SyncRun)}
}

func (r *InMemorySyncRunRepository) Create(_ context.Context, run *SyncRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *run
	r.runs[run.ID] = &cp
	return nil
}

func (r *InMemorySyncRunRepository) Update(_ context.Context, run *SyncRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.runs[run.ID]; !ok {
		return repository.ErrNotFound
	}
	cp := *run
	r.runs[run.ID] = &cp
	return nil
}

func (r *InMemorySyncRunRepository) FindByID(_ context.Context, teamID, id uuid.UUID) (*SyncRun, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	run, ok := r.runs[id]
	if !ok || run.TeamID != teamID {
		return nil, repository.ErrNotFound
	}
	cp := *run
	return &cp, nil
}

func (r *InMemorySyncRunRepository) List(_ context.Context, teamID uuid.UUID, filter SyncRunListFilter, page repository.PageParams) (repository.Page[SyncRun], error) {
	page = page.Normalize()
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]SyncRun, 0)
	for _, run := range r.runs {
		if run.TeamID != teamID {
			continue
		}
		if filter.ChannelID != nil && run.ChannelID != *filter.ChannelID {
			continue
		}
		items = append(items, *run)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].StartedAt.After(items[j].StartedAt)
	})

	total := int64(len(items))
	start := page.Offset()
	if start >= len(items) {
		return repository.Page[SyncRun]{Items: []SyncRun{}, Pagination: repository.NewMeta(page.Page, page.PageSize, total)}, nil
	}
	end := start + page.Limit()
	if end > len(items) {
		end = len(items)
	}
	return repository.Page[SyncRun]{
		Items:      items[start:end],
		Pagination: repository.NewMeta(page.Page, page.PageSize, total),
	}, nil
}

func (r *InMemorySyncRunRepository) LastSuccessfulByChannel(_ context.Context, channelID uuid.UUID) (*SyncRun, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var latest *SyncRun
	for _, run := range r.runs {
		if run.ChannelID != channelID || run.Status != SyncRunSuccess {
			continue
		}
		if latest == nil || run.StartedAt.After(latest.StartedAt) {
			cp := *run
			latest = &cp
		}
	}
	if latest == nil {
		return nil, repository.ErrNotFound
	}
	return latest, nil
}

func (r *InMemorySyncRunRepository) CountSuccessfulWithin(_ context.Context, teamID uuid.UUID, since time.Time) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var count int64
	for _, run := range r.runs {
		if run.TeamID == teamID && run.Status == SyncRunSuccess && run.StartedAt.After(since) {
			count++
		}
	}
	return count, nil
}

func (r *InMemorySyncRunRepository) MetricsByChannel(_ context.Context, teamID, channelID uuid.UUID) (*SyncMetrics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return aggregateMetrics(r.runs, teamID, &channelID), nil
}

func (r *InMemorySyncRunRepository) MetricsByTeam(_ context.Context, teamID uuid.UUID) (*SyncMetrics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return aggregateMetrics(r.runs, teamID, nil), nil
}

func aggregateMetrics(runs map[uuid.UUID]*SyncRun, teamID uuid.UUID, channelID *uuid.UUID) *SyncMetrics {
	m := &SyncMetrics{}
	var lastCompleted *time.Time
	for _, run := range runs {
		if run.TeamID != teamID {
			continue
		}
		if channelID != nil && run.ChannelID != *channelID {
			continue
		}
		m.SyncCount++
		if run.Status == SyncRunSuccess {
			m.SuccessCount++
			m.TemplatesSynced += int64(run.SyncedCount)
		}
		if run.Status == SyncRunFailed {
			m.FailureCount++
		}
		if run.CompletedAt != nil && (lastCompleted == nil || run.CompletedAt.After(*lastCompleted)) {
			lastCompleted = run.CompletedAt
			m.LastDurationMs = run.CompletedAt.Sub(run.StartedAt).Milliseconds()
		}
	}
	return m
}
