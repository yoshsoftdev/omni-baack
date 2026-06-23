package importjob

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// Repository (ImportRepository) persists contact import jobs, files, and row results.
type Repository interface {
	CreateJob(ctx context.Context, job *Job) error
	FindJobByID(ctx context.Context, teamID, id uuid.UUID) (*Job, error)
	ListJobs(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Job], error)
	UpdateJob(ctx context.Context, job *Job) error

	CreateFile(ctx context.Context, file *File) error
	ListFilesByJob(ctx context.Context, jobID uuid.UUID) ([]File, error)

	BulkCreateRows(ctx context.Context, rows []Row) error
	ListRowsByJob(ctx context.Context, jobID uuid.UUID, page repository.PageParams) (repository.Page[Row], error)
	CountRowsByStatus(ctx context.Context, jobID uuid.UUID) (map[RowStatus]int64, error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CreateJob(ctx context.Context, job *Job) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO contact_import_jobs (
			id, team_id, source, status, filename, total_rows, imported_count, updated_count,
			skipped_count, failed_count, error_summary, created_by, started_at, completed_at, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`, job.ID, job.TeamID, string(job.Source), string(job.Status), job.Filename, job.TotalRows,
		job.ImportedCount, job.UpdatedCount, job.SkippedCount, job.FailedCount, job.ErrorSummary,
		job.CreatedBy, job.StartedAt, job.CompletedAt, job.CreatedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) FindJobByID(ctx context.Context, teamID, id uuid.UUID) (*Job, error) {
	return scanJob(r.db.Pool.QueryRow(ctx, `
		SELECT id, team_id, source, status, filename, total_rows, imported_count, updated_count,
			skipped_count, failed_count, error_summary, created_by, started_at, completed_at, created_at
		FROM contact_import_jobs WHERE id = $1 AND team_id = $2
	`, id, teamID))
}

func (r *PostgresRepository) ListJobs(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Job], error) {
	page = page.Normalize()
	args := []any{teamID}
	where := `team_id = $1`
	if filter.Status != nil {
		args = append(args, string(*filter.Status))
		where += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	var total int64
	if err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM contact_import_jobs WHERE `+where, args...).Scan(&total); err != nil {
		return repository.Page[Job]{}, err
	}
	args = append(args, page.Limit(), page.Offset())
	rows, err := r.db.Pool.Query(ctx, fmt.Sprintf(`
		SELECT id, team_id, source, status, filename, total_rows, imported_count, updated_count,
			skipped_count, failed_count, error_summary, created_by, started_at, completed_at, created_at
		FROM contact_import_jobs WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d
	`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return repository.Page[Job]{}, err
	}
	defer rows.Close()
	items := make([]Job, 0)
	for rows.Next() {
		j, err := scanJobRow(rows)
		if err != nil {
			return repository.Page[Job]{}, err
		}
		items = append(items, *j)
	}
	return repository.Page[Job]{Items: items, Pagination: repository.NewMeta(page.Page, page.PageSize, total)}, rows.Err()
}

func (r *PostgresRepository) UpdateJob(ctx context.Context, job *Job) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE contact_import_jobs SET status=$1, total_rows=$2, imported_count=$3, updated_count=$4,
			skipped_count=$5, failed_count=$6, error_summary=$7, started_at=$8, completed_at=$9
		WHERE id=$10 AND team_id=$11
	`, string(job.Status), job.TotalRows, job.ImportedCount, job.UpdatedCount, job.SkippedCount,
		job.FailedCount, job.ErrorSummary, job.StartedAt, job.CompletedAt, job.ID, job.TeamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) CreateFile(ctx context.Context, file *File) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO contact_import_files (id, job_id, storage_key, mime_type, size_bytes, checksum, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, file.ID, file.JobID, file.StorageKey, file.MimeType, file.SizeBytes, file.Checksum, file.CreatedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) ListFilesByJob(ctx context.Context, jobID uuid.UUID) ([]File, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, job_id, storage_key, mime_type, size_bytes, checksum, created_at
		FROM contact_import_files WHERE job_id = $1 ORDER BY created_at ASC
	`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]File, 0)
	for rows.Next() {
		var f File
		if err := rows.Scan(&f.ID, &f.JobID, &f.StorageKey, &f.MimeType, &f.SizeBytes, &f.Checksum, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) BulkCreateRows(ctx context.Context, rows []Row) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		batch := &pgx.Batch{}
		for _, row := range rows {
			batch.Queue(`
				INSERT INTO contact_import_rows (id, job_id, row_number, status, contact_id, raw_data, error_message, created_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			`, row.ID, row.JobID, row.RowNumber, string(row.Status), row.ContactID, row.RawData, row.ErrorMessage, row.CreatedAt)
		}
		br := tx.SendBatch(ctx, batch)
		defer br.Close()
		for range rows {
			if _, err := br.Exec(); err != nil {
				return repository.MapError(err)
			}
		}
		return nil
	})
}

func (r *PostgresRepository) ListRowsByJob(ctx context.Context, jobID uuid.UUID, page repository.PageParams) (repository.Page[Row], error) {
	page = page.Normalize()
	var total int64
	if err := r.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM contact_import_rows WHERE job_id = $1`, jobID,
	).Scan(&total); err != nil {
		return repository.Page[Row]{}, err
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, job_id, row_number, status, contact_id, raw_data, error_message, created_at
		FROM contact_import_rows WHERE job_id = $1 ORDER BY row_number ASC LIMIT $2 OFFSET $3
	`, jobID, page.Limit(), page.Offset())
	if err != nil {
		return repository.Page[Row]{}, err
	}
	defer rows.Close()
	items := make([]Row, 0)
	for rows.Next() {
		row, err := scanRowRow(rows)
		if err != nil {
			return repository.Page[Row]{}, err
		}
		items = append(items, *row)
	}
	return repository.Page[Row]{Items: items, Pagination: repository.NewMeta(page.Page, page.PageSize, total)}, rows.Err()
}

func (r *PostgresRepository) CountRowsByStatus(ctx context.Context, jobID uuid.UUID) (map[RowStatus]int64, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT status, COUNT(*) FROM contact_import_rows WHERE job_id = $1 GROUP BY status
	`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[RowStatus]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		out[RowStatus(status)] = count
	}
	return out, rows.Err()
}

func scanJob(row pgx.Row) (*Job, error) { return scanJobRow(row) }

func scanJobRow(row pgx.Row) (*Job, error) {
	var j Job
	var source, status string
	err := row.Scan(
		&j.ID, &j.TeamID, &source, &status, &j.Filename, &j.TotalRows, &j.ImportedCount, &j.UpdatedCount,
		&j.SkippedCount, &j.FailedCount, &j.ErrorSummary, &j.CreatedBy, &j.StartedAt, &j.CompletedAt, &j.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan import job: %w", err)
	}
	j.Source = Source(source)
	j.Status = JobStatus(status)
	return &j, nil
}

func scanRowRow(row pgx.Row) (*Row, error) {
	var r Row
	var status string
	err := row.Scan(&r.ID, &r.JobID, &r.RowNumber, &status, &r.ContactID, &r.RawData, &r.ErrorMessage, &r.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan import row: %w", err)
	}
	r.Status = RowStatus(status)
	return &r, nil
}
