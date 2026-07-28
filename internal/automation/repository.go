package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type Repository interface {
	CreateEndpoint(ctx context.Context, item *Endpoint) error
	FindEndpointByID(ctx context.Context, teamID uuid.UUID, id string) (*Endpoint, error)
	FindEndpointByIDGlobal(ctx context.Context, id string) (*Endpoint, error)
	ListEndpoints(ctx context.Context, teamID uuid.UUID, filter EndpointListFilter, page repository.PageParams) (repository.Page[Endpoint], error)
	UpdateEndpoint(ctx context.Context, item *Endpoint) error
	SoftDeleteEndpoint(ctx context.Context, teamID uuid.UUID, id string) error
	TouchEndpointUsed(ctx context.Context, id string, at time.Time) error
	SlugExists(ctx context.Context, teamID uuid.UUID, slug string, excludeID string) (bool, error)

	CreateAPIKey(ctx context.Context, item *APIKey) error
	FindAPIKeyByID(ctx context.Context, teamID uuid.UUID, id string) (*APIKey, error)
	FindAPIKeyByHash(ctx context.Context, hash string) (*APIKey, error)
	ListAPIKeys(ctx context.Context, teamID uuid.UUID) ([]APIKey, error)
	UpdateAPIKey(ctx context.Context, item *APIKey) error
	SoftDeleteAPIKey(ctx context.Context, teamID uuid.UUID, id string) error
	TouchAPIKeyUsed(ctx context.Context, id string, at time.Time) error
	CountActiveAPIKeys(ctx context.Context, teamID uuid.UUID) (int64, error)

	CreateExecution(ctx context.Context, item *Execution) error
	CreateWebhookDelivery(ctx context.Context, item *WebhookDelivery) error

	CreateForwardWebhook(ctx context.Context, item *ForwardWebhook) error
	FindForwardWebhookByID(ctx context.Context, teamID uuid.UUID, id string) (*ForwardWebhook, error)
	ListForwardWebhooks(ctx context.Context, teamID uuid.UUID) ([]ForwardWebhook, error)
	ListActiveForwardWebhooks(ctx context.Context, teamID uuid.UUID) ([]ForwardWebhook, error)
	UpdateForwardWebhook(ctx context.Context, item *ForwardWebhook) error
	SoftDeleteForwardWebhook(ctx context.Context, teamID uuid.UUID, id string) error
	TouchForwardWebhookUsed(ctx context.Context, id string, at time.Time) error
	CreateForwardDelivery(ctx context.Context, item *ForwardDelivery) error
	ListForwardDeliveries(ctx context.Context, teamID uuid.UUID, webhookID string, page repository.PageParams) (repository.Page[ForwardDelivery], error)

	GetSummary(ctx context.Context, teamID uuid.UUID) (*Summary, error)
	GetEndpointStats(ctx context.Context, endpointID string) (EndpointStats, error)
	GetEndpointAnalytics(ctx context.Context, teamID uuid.UUID, endpointID string) (*EndpointAnalytics, error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CreateEndpoint(ctx context.Context, item *Endpoint) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO automation_endpoints (
			id, team_id, name, slug, description, method, status, action_type, action_config,
			webhook_url, webhook_enabled, webhook_secret,
			created_by, created_at, updated_at, last_used_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
	`, item.ID, item.TeamID, item.Name, item.Slug, item.Description, item.Method, item.Status,
		item.ActionType, item.ActionConfig, item.Webhook.URL, item.Webhook.IsEnabled, item.Webhook.Secret,
		item.CreatedBy, item.CreatedAt, item.UpdatedAt, item.LastUsedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) FindEndpointByID(ctx context.Context, teamID uuid.UUID, id string) (*Endpoint, error) {
	ep, err := scanEndpoint(r.db.Pool.QueryRow(ctx, endpointSelect+`
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, id, teamID))
	if err != nil {
		return nil, err
	}
	stats, _ := r.GetEndpointStats(ctx, ep.ID)
	ep.Stats = stats
	return ep, nil
}

func (r *PostgresRepository) FindEndpointByIDGlobal(ctx context.Context, id string) (*Endpoint, error) {
	ep, err := scanEndpoint(r.db.Pool.QueryRow(ctx, endpointSelect+`
		WHERE id = $1 AND deleted_at IS NULL
	`, id))
	if err != nil {
		return nil, err
	}
	stats, _ := r.GetEndpointStats(ctx, ep.ID)
	ep.Stats = stats
	return ep, nil
}

const endpointSelect = `
	SELECT id, team_id, name, slug, description, method, status, action_type, action_config,
	       webhook_url, webhook_enabled, webhook_secret,
	       created_by, created_at, updated_at, deleted_at, last_used_at
	FROM automation_endpoints
`

func (r *PostgresRepository) ListEndpoints(ctx context.Context, teamID uuid.UUID, filter EndpointListFilter, page repository.PageParams) (repository.Page[Endpoint], error) {
	page = page.Normalize()
	args := []any{teamID}
	where := `team_id = $1 AND deleted_at IS NULL`
	if filter.Status != "" && filter.Status != "all" {
		args = append(args, filter.Status)
		where += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	if filter.ActionType != "" && filter.ActionType != "all" {
		args = append(args, filter.ActionType)
		where += fmt.Sprintf(` AND action_type = $%d`, len(args))
	}
	if strings.TrimSpace(filter.Search) != "" {
		args = append(args, "%"+strings.TrimSpace(filter.Search)+"%")
		where += fmt.Sprintf(` AND (name ILIKE $%d OR slug ILIKE $%d OR id ILIKE $%d)`, len(args), len(args), len(args))
	}

	var total int64
	if err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM automation_endpoints WHERE `+where, args...).Scan(&total); err != nil {
		return repository.Page[Endpoint]{}, fmt.Errorf("count automation endpoints: %w", err)
	}

	args = append(args, page.Limit(), page.Offset())
	rows, err := r.db.Pool.Query(ctx, fmt.Sprintf(`
		%s WHERE %s
		ORDER BY updated_at DESC
		LIMIT $%d OFFSET $%d
	`, endpointSelect, where, len(args)-1, len(args)), args...)
	if err != nil {
		return repository.Page[Endpoint]{}, fmt.Errorf("list automation endpoints: %w", err)
	}
	defer rows.Close()

	items := make([]Endpoint, 0)
	for rows.Next() {
		item, err := scanEndpointRow(rows)
		if err != nil {
			return repository.Page[Endpoint]{}, err
		}
		stats, _ := r.GetEndpointStats(ctx, item.ID)
		item.Stats = stats
		items = append(items, *item)
	}
	return repository.Page[Endpoint]{Items: items, Pagination: repository.NewMeta(page.Page, page.PageSize, total)}, rows.Err()
}

func (r *PostgresRepository) UpdateEndpoint(ctx context.Context, item *Endpoint) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE automation_endpoints
		SET name = $3, slug = $4, description = $5, status = $6, action_config = $7,
		    webhook_url = $8, webhook_enabled = $9, webhook_secret = $10, updated_at = $11
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, item.ID, item.TeamID, item.Name, item.Slug, item.Description, item.Status, item.ActionConfig,
		item.Webhook.URL, item.Webhook.IsEnabled, item.Webhook.Secret, item.UpdatedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) SoftDeleteEndpoint(ctx context.Context, teamID uuid.UUID, id string) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE automation_endpoints SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, id, teamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) TouchEndpointUsed(ctx context.Context, id string, at time.Time) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE automation_endpoints SET last_used_at = $2, updated_at = $2 WHERE id = $1 AND deleted_at IS NULL
	`, id, at)
	return err
}

func (r *PostgresRepository) SlugExists(ctx context.Context, teamID uuid.UUID, slug string, excludeID string) (bool, error) {
	var exists bool
	err := r.db.Pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM automation_endpoints
			WHERE team_id = $1 AND slug = $2 AND deleted_at IS NULL AND ($3 = '' OR id <> $3)
		)
	`, teamID, slug, excludeID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) CreateAPIKey(ctx context.Context, item *APIKey) error {
	scopes, err := json.Marshal(item.Scopes)
	if err != nil {
		return err
	}
	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO automation_api_keys (
			id, team_id, name, key_prefix, key_hash, scopes, status, created_by, created_at, updated_at, last_used_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`, item.ID, item.TeamID, item.Name, item.KeyPrefix, item.KeyHash, scopes, item.Status,
		item.CreatedBy, item.CreatedAt, item.UpdatedAt, item.LastUsedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) FindAPIKeyByID(ctx context.Context, teamID uuid.UUID, id string) (*APIKey, error) {
	return scanAPIKey(r.db.Pool.QueryRow(ctx, apiKeySelect+`
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, id, teamID))
}

func (r *PostgresRepository) FindAPIKeyByHash(ctx context.Context, hash string) (*APIKey, error) {
	return scanAPIKey(r.db.Pool.QueryRow(ctx, apiKeySelect+`
		WHERE key_hash = $1 AND deleted_at IS NULL
	`, hash))
}

const apiKeySelect = `
	SELECT id, team_id, name, key_prefix, key_hash, scopes, status, created_by, created_at, updated_at, deleted_at, last_used_at
	FROM automation_api_keys
`

func (r *PostgresRepository) ListAPIKeys(ctx context.Context, teamID uuid.UUID) ([]APIKey, error) {
	rows, err := r.db.Pool.Query(ctx, apiKeySelect+`
		WHERE team_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]APIKey, 0)
	for rows.Next() {
		item, err := scanAPIKeyRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) UpdateAPIKey(ctx context.Context, item *APIKey) error {
	scopes, err := json.Marshal(item.Scopes)
	if err != nil {
		return err
	}
	_, err = r.db.Pool.Exec(ctx, `
		UPDATE automation_api_keys
		SET name = $3, key_prefix = $4, key_hash = $5, scopes = $6, status = $7, updated_at = $8
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, item.ID, item.TeamID, item.Name, item.KeyPrefix, item.KeyHash, scopes, item.Status, item.UpdatedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) SoftDeleteAPIKey(ctx context.Context, teamID uuid.UUID, id string) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE automation_api_keys SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, id, teamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) TouchAPIKeyUsed(ctx context.Context, id string, at time.Time) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE automation_api_keys SET last_used_at = $2, updated_at = $2 WHERE id = $1 AND deleted_at IS NULL
	`, id, at)
	return err
}

func (r *PostgresRepository) CountActiveAPIKeys(ctx context.Context, teamID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM automation_api_keys WHERE team_id = $1 AND status = 'active' AND deleted_at IS NULL
	`, teamID).Scan(&count)
	return count, err
}

func (r *PostgresRepository) CreateExecution(ctx context.Context, item *Execution) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO automation_executions (
			id, team_id, endpoint_id, api_key_id, status, status_code, latency_ms, error_message, client_ip, provider_message_id, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`, item.ID, item.TeamID, item.EndpointID, item.APIKeyID, item.Status, item.StatusCode, item.LatencyMs,
		item.ErrorMessage, item.ClientIP, item.ProviderMessageID, item.CreatedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) CreateWebhookDelivery(ctx context.Context, item *WebhookDelivery) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO automation_webhook_deliveries (
			id, team_id, endpoint_id, execution_id, status, status_code, error_message, attempted_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, item.ID, item.TeamID, item.EndpointID, item.ExecutionID, item.Status, item.StatusCode, item.ErrorMessage, item.AttemptedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) CreateForwardWebhook(ctx context.Context, item *ForwardWebhook) error {
	events, err := json.Marshal(item.Events)
	if err != nil {
		return err
	}
	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO automation_forward_webhooks (
			id, team_id, name, url, secret, status, events, channel_id, created_by, created_at, updated_at, last_used_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, item.ID, item.TeamID, item.Name, item.URL, item.Secret, item.Status, events, item.ChannelID,
		item.CreatedBy, item.CreatedAt, item.UpdatedAt, item.LastUsedAt)
	return repository.MapError(err)
}

const forwardWebhookSelect = `
	SELECT id, team_id, name, url, secret, status, events, channel_id, created_by, created_at, updated_at, deleted_at, last_used_at
	FROM automation_forward_webhooks
`

func (r *PostgresRepository) FindForwardWebhookByID(ctx context.Context, teamID uuid.UUID, id string) (*ForwardWebhook, error) {
	return scanForwardWebhook(r.db.Pool.QueryRow(ctx, forwardWebhookSelect+`
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, id, teamID))
}

func (r *PostgresRepository) ListForwardWebhooks(ctx context.Context, teamID uuid.UUID) ([]ForwardWebhook, error) {
	rows, err := r.db.Pool.Query(ctx, forwardWebhookSelect+`
		WHERE team_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ForwardWebhook, 0)
	for rows.Next() {
		item, err := scanForwardWebhookRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) ListActiveForwardWebhooks(ctx context.Context, teamID uuid.UUID) ([]ForwardWebhook, error) {
	rows, err := r.db.Pool.Query(ctx, forwardWebhookSelect+`
		WHERE team_id = $1 AND status = 'active' AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ForwardWebhook, 0)
	for rows.Next() {
		item, err := scanForwardWebhookRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) UpdateForwardWebhook(ctx context.Context, item *ForwardWebhook) error {
	events, err := json.Marshal(item.Events)
	if err != nil {
		return err
	}
	_, err = r.db.Pool.Exec(ctx, `
		UPDATE automation_forward_webhooks
		SET name = $3, url = $4, secret = $5, status = $6, events = $7, channel_id = $8, updated_at = $9
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, item.ID, item.TeamID, item.Name, item.URL, item.Secret, item.Status, events, item.ChannelID, item.UpdatedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) SoftDeleteForwardWebhook(ctx context.Context, teamID uuid.UUID, id string) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE automation_forward_webhooks SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, id, teamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) TouchForwardWebhookUsed(ctx context.Context, id string, at time.Time) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE automation_forward_webhooks SET last_used_at = $2, updated_at = $2
		WHERE id = $1 AND deleted_at IS NULL
	`, id, at)
	return err
}

func (r *PostgresRepository) CreateForwardDelivery(ctx context.Context, item *ForwardDelivery) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO automation_forward_deliveries (
			id, team_id, webhook_id, event_type, message_id, conversation_id, status, status_code, error_message, attempted_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, item.ID, item.TeamID, item.WebhookID, item.EventType, item.MessageID, item.ConversationID,
		item.Status, item.StatusCode, item.ErrorMessage, item.AttemptedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) ListForwardDeliveries(ctx context.Context, teamID uuid.UUID, webhookID string, page repository.PageParams) (repository.Page[ForwardDelivery], error) {
	page = page.Normalize()
	var total int64
	if err := r.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM automation_forward_deliveries WHERE team_id = $1 AND webhook_id = $2
	`, teamID, webhookID).Scan(&total); err != nil {
		return repository.Page[ForwardDelivery]{}, err
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, team_id, webhook_id, event_type, message_id, conversation_id, status, status_code, error_message, attempted_at
		FROM automation_forward_deliveries
		WHERE team_id = $1 AND webhook_id = $2
		ORDER BY attempted_at DESC
		LIMIT $3 OFFSET $4
	`, teamID, webhookID, page.Limit(), page.Offset())
	if err != nil {
		return repository.Page[ForwardDelivery]{}, err
	}
	defer rows.Close()
	items := make([]ForwardDelivery, 0)
	for rows.Next() {
		var item ForwardDelivery
		if err := rows.Scan(
			&item.ID, &item.TeamID, &item.WebhookID, &item.EventType, &item.MessageID, &item.ConversationID,
			&item.Status, &item.StatusCode, &item.ErrorMessage, &item.AttemptedAt,
		); err != nil {
			return repository.Page[ForwardDelivery]{}, err
		}
		items = append(items, item)
	}
	return repository.Page[ForwardDelivery]{
		Items:      items,
		Pagination: repository.NewMeta(page.Page, page.PageSize, total),
	}, rows.Err()
}

func scanForwardWebhook(row pgx.Row) (*ForwardWebhook, error) {
	var item ForwardWebhook
	var events rawScopes
	err := row.Scan(
		&item.ID, &item.TeamID, &item.Name, &item.URL, &item.Secret, &item.Status, &events, &item.ChannelID,
		&item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt, &item.LastUsedAt,
	)
	if err != nil {
		return nil, repository.MapError(err)
	}
	item.Events = events
	return &item, nil
}

func scanForwardWebhookRow(rows pgx.Rows) (*ForwardWebhook, error) {
	var item ForwardWebhook
	var events rawScopes
	err := rows.Scan(
		&item.ID, &item.TeamID, &item.Name, &item.URL, &item.Secret, &item.Status, &events, &item.ChannelID,
		&item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt, &item.LastUsedAt,
	)
	if err != nil {
		return nil, err
	}
	item.Events = events
	return &item, nil
}

func (r *PostgresRepository) GetSummary(ctx context.Context, teamID uuid.UUID) (*Summary, error) {
	summary := &Summary{}
	if err := r.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM automation_endpoints WHERE team_id = $1 AND deleted_at IS NULL
	`, teamID).Scan(&summary.EndpointCount); err != nil {
		return nil, err
	}
	if err := r.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM automation_api_keys WHERE team_id = $1 AND status = 'active' AND deleted_at IS NULL
	`, teamID).Scan(&summary.APIKeyCount); err != nil {
		return nil, err
	}

	var successToday, failedToday int64
	_ = r.db.Pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'success'),
			COUNT(*) FILTER (WHERE status = 'failed')
		FROM automation_executions
		WHERE team_id = $1 AND created_at >= date_trunc('day', NOW() AT TIME ZONE 'UTC')
	`, teamID).Scan(&successToday, &failedToday)
	summary.RequestsToday = successToday + failedToday
	summary.FailedToday = failedToday
	if summary.RequestsToday > 0 {
		summary.SuccessRate = float64(successToday) * 100 / float64(summary.RequestsToday)
		summary.SuccessRate = float64(int(summary.SuccessRate*10+0.5)) / 10
	} else {
		summary.SuccessRate = 100
	}
	return summary, nil
}

func (r *PostgresRepository) GetEndpointStats(ctx context.Context, endpointID string) (EndpointStats, error) {
	var stats EndpointStats
	var success, failed, successToday, failedToday int64
	var avgLatency *float64
	err := r.db.Pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'success'),
			COUNT(*) FILTER (WHERE status = 'failed'),
			COUNT(*) FILTER (WHERE status = 'success' AND created_at >= date_trunc('day', NOW() AT TIME ZONE 'UTC')),
			COUNT(*) FILTER (WHERE status = 'failed' AND created_at >= date_trunc('day', NOW() AT TIME ZONE 'UTC')),
			AVG(latency_ms) FILTER (WHERE status IN ('success','failed'))
		FROM automation_executions
		WHERE endpoint_id = $1
	`, endpointID).Scan(&success, &failed, &successToday, &failedToday, &avgLatency)
	if err != nil {
		return stats, err
	}
	stats.TotalCalls = success + failed
	stats.FailedCalls = failed
	stats.CallsToday = successToday + failedToday
	if stats.TotalCalls > 0 {
		stats.SuccessRate = float64(success) * 100 / float64(stats.TotalCalls)
		stats.SuccessRate = float64(int(stats.SuccessRate*10+0.5)) / 10
	} else {
		stats.SuccessRate = 100
	}
	if avgLatency != nil {
		stats.AverageLatencyMs = int64(*avgLatency + 0.5)
	}
	return stats, nil
}

func (r *PostgresRepository) GetEndpointAnalytics(ctx context.Context, teamID uuid.UUID, endpointID string) (*EndpointAnalytics, error) {
	stats, err := r.GetEndpointStats(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	ep, err := r.FindEndpointByID(ctx, teamID, endpointID)
	if err != nil {
		return nil, err
	}
	out := &EndpointAnalytics{
		EndpointID:       endpointID,
		CallsToday:       stats.CallsToday,
		TotalRequests:    stats.TotalCalls,
		SuccessRate:      stats.SuccessRate,
		FailedRequests:   stats.FailedCalls,
		AverageLatencyMs: stats.AverageLatencyMs,
		LastUsedAt:       ep.LastUsedAt,
		TopClientIPs:     []ClientIPCount{},
		RequestsOverTime: []DayRequestStat{},
	}

	var errAt *time.Time
	var errMsg *string
	var errCode int
	_ = r.db.Pool.QueryRow(ctx, `
		SELECT created_at, error_message, status_code
		FROM automation_executions
		WHERE endpoint_id = $1 AND status = 'failed'
		ORDER BY created_at DESC
		LIMIT 1
	`, endpointID).Scan(&errAt, &errMsg, &errCode)
	if errAt != nil && errMsg != nil {
		out.LastError = &LastError{At: *errAt, Message: *errMsg, StatusCode: errCode}
	}

	rows, err := r.db.Pool.Query(ctx, `
		SELECT COALESCE(client_ip, 'unknown') AS ip, COUNT(*) AS cnt
		FROM automation_executions
		WHERE endpoint_id = $1 AND client_ip IS NOT NULL AND client_ip <> ''
		GROUP BY 1
		ORDER BY cnt DESC
		LIMIT 5
	`, endpointID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var row ClientIPCount
			if err := rows.Scan(&row.IP, &row.Count); err != nil {
				return nil, err
			}
			out.TopClientIPs = append(out.TopClientIPs, row)
		}
	}

	dayRows, err := r.db.Pool.Query(ctx, `
		SELECT to_char(date_trunc('day', created_at), 'Mon DD') AS day_label,
		       COUNT(*) FILTER (WHERE status = 'success') AS success,
		       COUNT(*) FILTER (WHERE status = 'failed') AS failed
		FROM automation_executions
		WHERE endpoint_id = $1 AND created_at >= NOW() - INTERVAL '7 days'
		GROUP BY date_trunc('day', created_at)
		ORDER BY date_trunc('day', created_at)
	`, endpointID)
	if err == nil {
		defer dayRows.Close()
		for dayRows.Next() {
			var row DayRequestStat
			if err := dayRows.Scan(&row.Date, &row.Success, &row.Failed); err != nil {
				return nil, err
			}
			out.RequestsOverTime = append(out.RequestsOverTime, row)
		}
	}
	return out, nil
}

func scanEndpoint(row pgx.Row) (*Endpoint, error) {
	var item Endpoint
	err := row.Scan(
		&item.ID, &item.TeamID, &item.Name, &item.Slug, &item.Description, &item.Method, &item.Status,
		&item.ActionType, &item.ActionConfig, &item.Webhook.URL, &item.Webhook.IsEnabled, &item.Webhook.Secret,
		&item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt, &item.LastUsedAt,
	)
	if err != nil {
		return nil, repository.MapError(err)
	}
	return &item, nil
}

func scanEndpointRow(rows pgx.Rows) (*Endpoint, error) {
	var item Endpoint
	err := rows.Scan(
		&item.ID, &item.TeamID, &item.Name, &item.Slug, &item.Description, &item.Method, &item.Status,
		&item.ActionType, &item.ActionConfig, &item.Webhook.URL, &item.Webhook.IsEnabled, &item.Webhook.Secret,
		&item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt, &item.LastUsedAt,
	)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func scanAPIKey(row pgx.Row) (*APIKey, error) {
	var item APIKey
	var scopes rawScopes
	err := row.Scan(
		&item.ID, &item.TeamID, &item.Name, &item.KeyPrefix, &item.KeyHash, &scopes, &item.Status,
		&item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt, &item.LastUsedAt,
	)
	if err != nil {
		return nil, repository.MapError(err)
	}
	item.Scopes = scopes
	return &item, nil
}

func scanAPIKeyRow(rows pgx.Rows) (*APIKey, error) {
	var item APIKey
	var scopes rawScopes
	err := rows.Scan(
		&item.ID, &item.TeamID, &item.Name, &item.KeyPrefix, &item.KeyHash, &scopes, &item.Status,
		&item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt, &item.LastUsedAt,
	)
	if err != nil {
		return nil, err
	}
	item.Scopes = scopes
	return &item, nil
}

type rawScopes []string

func (s *rawScopes) Scan(src any) error {
	if src == nil {
		*s = []string{}
		return nil
	}
	switch v := src.(type) {
	case []byte:
		return json.Unmarshal(v, s)
	case string:
		return json.Unmarshal([]byte(v), s)
	default:
		return fmt.Errorf("unsupported scopes type %T", src)
	}
}
