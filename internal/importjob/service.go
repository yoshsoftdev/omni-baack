package importjob

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/quota"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// Service (ImportService) manages contact import jobs.
type Service struct {
	imports   Repository
	teams     team.Repository
	quota     *quota.Service
	rbac      *rbac.Service
}

func NewService(
	imports Repository,
	teams team.Repository,
	quotaSvc *quota.Service,
	rbacSvc *rbac.Service,
) *Service {
	return &Service{
		imports: imports,
		teams:   teams,
		quota:   quotaSvc,
		rbac:    rbacSvc,
	}
}

type CreateJobInput struct {
	Source     Source
	Filename   string
	TotalRows  int
	StorageKey string
	MimeType   string
	SizeBytes  int64
	Checksum   string
}

type CreateFileInput struct {
	StorageKey string
	MimeType   string
	SizeBytes  int64
	Checksum   string
}

func (s *Service) authorize(role, perm string) error {
	if s.rbac != nil && s.rbac.Require(role, perm) {
		return nil
	}
	return serviceerrors.ErrForbidden
}

func (s *Service) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	if err != nil {
		if err == team.ErrMemberNotFound {
			return serviceerrors.ErrForbidden
		}
		return err
	}
	return nil
}

func (s *Service) CreateJob(ctx context.Context, teamID, userID uuid.UUID, actorRole string, input CreateJobInput) (*Job, error) {
	if err := s.authorize(actorRole, rbac.PermContactCreate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	filename := strings.TrimSpace(input.Filename)
	if filename == "" {
		return nil, serviceerrors.NewValidation("filename", "is required")
	}
	if input.TotalRows < 0 {
		return nil, serviceerrors.NewValidation("total_rows", "must be non-negative")
	}
	if err := s.quota.AssertContactHeadroom(ctx, teamID, input.TotalRows); err != nil {
		return nil, err
	}
	switch input.Source {
	case SourceCSV, SourceExcel, SourceManual:
	default:
		return nil, serviceerrors.NewValidation("source", "invalid value")
	}

	now := time.Now().UTC()
	job := &Job{
		ID:        uuid.New(),
		TeamID:    teamID,
		Source:    input.Source,
		Status:    StatusPending,
		Filename:  filename,
		TotalRows: input.TotalRows,
		CreatedBy: &userID,
		CreatedAt: now,
	}
	if err := serviceerrors.MapRepository(s.imports.CreateJob(ctx, job)); err != nil {
		return nil, err
	}

	if input.StorageKey != "" {
		file := &File{
			ID:         uuid.New(),
			JobID:      job.ID,
			StorageKey: input.StorageKey,
			MimeType:   input.MimeType,
			SizeBytes:  input.SizeBytes,
			Checksum:   input.Checksum,
			CreatedAt:  now,
		}
		if err := serviceerrors.MapRepository(s.imports.CreateFile(ctx, file)); err != nil {
			return nil, err
		}
	}
	return job, nil
}

func (s *Service) GetJob(ctx context.Context, teamID, userID uuid.UUID, actorRole string, jobID uuid.UUID) (*Job, error) {
	if err := s.authorize(actorRole, rbac.PermContactView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	job, err := s.imports.FindJobByID(ctx, teamID, jobID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return job, nil
}

func (s *Service) ListJobs(ctx context.Context, teamID, userID uuid.UUID, actorRole string, filter ListFilter, page repository.PageParams) (repository.Page[Job], error) {
	if err := s.authorize(actorRole, rbac.PermContactView); err != nil {
		return repository.Page[Job]{}, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return repository.Page[Job]{}, err
	}
	result, err := s.imports.ListJobs(ctx, teamID, filter, page)
	if err != nil {
		return repository.Page[Job]{}, serviceerrors.MapRepository(err)
	}
	return result, nil
}

func (s *Service) MarkProcessing(ctx context.Context, teamID, jobID uuid.UUID) error {
	job, err := s.imports.FindJobByID(ctx, teamID, jobID)
	if err != nil {
		return serviceerrors.MapRepository(err)
	}
	if job.Status != StatusPending {
		return serviceerrors.NewValidation("status", "job is not pending")
	}
	now := time.Now().UTC()
	job.Status = StatusProcessing
	job.StartedAt = &now
	return serviceerrors.MapRepository(s.imports.UpdateJob(ctx, job))
}

func (s *Service) CompleteJob(ctx context.Context, teamID, jobID uuid.UUID, imported, updated, skipped, failed int, errorSummary *string) error {
	job, err := s.imports.FindJobByID(ctx, teamID, jobID)
	if err != nil {
		return serviceerrors.MapRepository(err)
	}
	now := time.Now().UTC()
	job.Status = StatusCompleted
	if failed > 0 && imported == 0 && updated == 0 {
		job.Status = StatusFailed
	}
	job.ImportedCount = imported
	job.UpdatedCount = updated
	job.SkippedCount = skipped
	job.FailedCount = failed
	job.ErrorSummary = errorSummary
	job.CompletedAt = &now
	return serviceerrors.MapRepository(s.imports.UpdateJob(ctx, job))
}

func (s *Service) AddFile(ctx context.Context, teamID uuid.UUID, jobID uuid.UUID, input CreateFileInput) (*File, error) {
	if _, err := s.imports.FindJobByID(ctx, teamID, jobID); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	now := time.Now().UTC()
	file := &File{
		ID:         uuid.New(),
		JobID:      jobID,
		StorageKey: input.StorageKey,
		MimeType:   input.MimeType,
		SizeBytes:  input.SizeBytes,
		Checksum:   input.Checksum,
		CreatedAt:  now,
	}
	if err := serviceerrors.MapRepository(s.imports.CreateFile(ctx, file)); err != nil {
		return nil, err
	}
	return file, nil
}

func (s *Service) RecordRows(ctx context.Context, rows []Row) error {
	if len(rows) == 0 {
		return nil
	}
	return serviceerrors.MapRepository(s.imports.BulkCreateRows(ctx, rows))
}
