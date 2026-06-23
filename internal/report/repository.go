package report

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// Repository (ReportRepository) reads and writes campaign reporting aggregates.
type Repository interface {
	AggregateDashboard(ctx context.Context, teamID uuid.UUID, range_ DateRange, channelID *uuid.UUID) (*DashboardStats, error)
	ListDailyStats(ctx context.Context, teamID uuid.UUID, range_ DateRange) ([]DailyStat, error)
	IncrementDailyStat(ctx context.Context, teamID uuid.UUID, statDate time.Time, delta DailyStatDelta) error

	GetCampaignFunnel(ctx context.Context, teamID, campaignID uuid.UUID) (*CampaignFunnel, error)
	ListCampaignFunnels(ctx context.Context, teamID uuid.UUID, range_ DateRange, page repository.PageParams) (repository.Page[CampaignFunnel], error)

	RecordLinkClick(ctx context.Context, click *LinkClick) error
	ListLinkClicks(ctx context.Context, campaignID uuid.UUID, page repository.PageParams) (repository.Page[LinkClick], error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) AggregateDashboard(ctx context.Context, teamID uuid.UUID, range_ DateRange, channelID *uuid.UUID) (*DashboardStats, error) {
	if channelID != nil {
		var stats DashboardStats
		err := r.db.Pool.QueryRow(ctx, `
			SELECT
				COUNT(DISTINCT c.id),
				COALESCE(SUM(c.sent_count), 0),
				COALESCE(SUM(c.delivered_count), 0),
				COALESCE(SUM(c.read_count), 0),
				COALESCE(SUM(c.failed_count), 0),
				COALESCE(SUM(c.reply_count), 0),
				COALESCE(SUM(c.clicked_count), 0)
			FROM campaigns c
			WHERE c.team_id = $1 AND c.deleted_at IS NULL AND c.channel_id = $2
				AND c.created_at >= $3 AND c.created_at <= $4
		`, teamID, *channelID, range_.From, range_.To).Scan(
			&stats.TotalCampaigns, &stats.MessagesSent, &stats.MessagesDelivered,
			&stats.MessagesRead, &stats.MessagesFailed, &stats.RepliesReceived, &stats.LinkClicks,
		)
		if err != nil {
			return nil, err
		}
		return &stats, nil
	}
	var stats DashboardStats
	err := r.db.Pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(campaigns_started), 0),
			COALESCE(SUM(messages_sent), 0),
			COALESCE(SUM(messages_delivered), 0),
			COALESCE(SUM(messages_read), 0),
			COALESCE(SUM(messages_failed), 0),
			COALESCE(SUM(replies_received), 0),
			COALESCE(SUM(link_clicks), 0)
		FROM campaign_daily_stats
		WHERE team_id = $1 AND stat_date >= $2::date AND stat_date <= $3::date
	`, teamID, range_.From, range_.To).Scan(
		&stats.TotalCampaigns, &stats.MessagesSent, &stats.MessagesDelivered,
		&stats.MessagesRead, &stats.MessagesFailed, &stats.RepliesReceived, &stats.LinkClicks,
	)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

func (r *PostgresRepository) ListDailyStats(ctx context.Context, teamID uuid.UUID, range_ DateRange) ([]DailyStat, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, team_id, stat_date, campaigns_started, messages_sent, messages_delivered,
			messages_read, messages_failed, replies_received, link_clicks, created_at, updated_at
		FROM campaign_daily_stats
		WHERE team_id = $1 AND stat_date >= $2::date AND stat_date <= $3::date
		ORDER BY stat_date ASC
	`, teamID, range_.From, range_.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]DailyStat, 0)
	for rows.Next() {
		var s DailyStat
		if err := rows.Scan(
			&s.ID, &s.TeamID, &s.StatDate, &s.CampaignsStarted, &s.MessagesSent, &s.MessagesDelivered,
			&s.MessagesRead, &s.MessagesFailed, &s.RepliesReceived, &s.LinkClicks, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) IncrementDailyStat(ctx context.Context, teamID uuid.UUID, statDate time.Time, delta DailyStatDelta) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO campaign_daily_stats (
			team_id, stat_date, campaigns_started, messages_sent, messages_delivered,
			messages_read, messages_failed, replies_received, link_clicks, created_at, updated_at
		) VALUES ($1, $2::date, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
		ON CONFLICT (team_id, stat_date) DO UPDATE SET
			campaigns_started = campaign_daily_stats.campaigns_started + EXCLUDED.campaigns_started,
			messages_sent = campaign_daily_stats.messages_sent + EXCLUDED.messages_sent,
			messages_delivered = campaign_daily_stats.messages_delivered + EXCLUDED.messages_delivered,
			messages_read = campaign_daily_stats.messages_read + EXCLUDED.messages_read,
			messages_failed = campaign_daily_stats.messages_failed + EXCLUDED.messages_failed,
			replies_received = campaign_daily_stats.replies_received + EXCLUDED.replies_received,
			link_clicks = campaign_daily_stats.link_clicks + EXCLUDED.link_clicks,
			updated_at = NOW()
	`, teamID, statDate, delta.CampaignsStarted, delta.MessagesSent, delta.MessagesDelivered,
		delta.MessagesRead, delta.MessagesFailed, delta.RepliesReceived, delta.LinkClicks)
	return repository.MapError(err)
}

func (r *PostgresRepository) GetCampaignFunnel(ctx context.Context, teamID, campaignID uuid.UUID) (*CampaignFunnel, error) {
	return scanFunnel(r.db.Pool.QueryRow(ctx, `
		SELECT id, team_id, name, status::text, total_contacts, sent_count, delivered_count,
			read_count, failed_count, reply_count, clicked_count, started_at, completed_at
		FROM campaigns
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, campaignID, teamID))
}

func (r *PostgresRepository) ListCampaignFunnels(ctx context.Context, teamID uuid.UUID, range_ DateRange, page repository.PageParams) (repository.Page[CampaignFunnel], error) {
	page = page.Normalize()
	args := []any{teamID, range_.From, range_.To}
	where := `team_id = $1 AND deleted_at IS NULL AND created_at >= $2 AND created_at <= $3`
	var total int64
	if err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM campaigns WHERE `+where, args...).Scan(&total); err != nil {
		return repository.Page[CampaignFunnel]{}, err
	}
	args = append(args, page.Limit(), page.Offset())
	rows, err := r.db.Pool.Query(ctx, fmt.Sprintf(`
		SELECT id, team_id, name, status::text, total_contacts, sent_count, delivered_count,
			read_count, failed_count, reply_count, clicked_count, started_at, completed_at
		FROM campaigns WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d
	`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return repository.Page[CampaignFunnel]{}, err
	}
	defer rows.Close()
	items := make([]CampaignFunnel, 0)
	for rows.Next() {
		f, err := scanFunnelRow(rows)
		if err != nil {
			return repository.Page[CampaignFunnel]{}, err
		}
		items = append(items, *f)
	}
	return repository.Page[CampaignFunnel]{Items: items, Pagination: repository.NewMeta(page.Page, page.PageSize, total)}, rows.Err()
}

func (r *PostgresRepository) RecordLinkClick(ctx context.Context, click *LinkClick) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO campaign_link_clicks (id, campaign_id, recipient_id, team_id, url, clicked_at, user_agent, device_type)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, click.ID, click.CampaignID, click.RecipientID, click.TeamID, click.URL,
		click.ClickedAt, click.UserAgent, click.DeviceType)
	return repository.MapError(err)
}

func (r *PostgresRepository) ListLinkClicks(ctx context.Context, campaignID uuid.UUID, page repository.PageParams) (repository.Page[LinkClick], error) {
	page = page.Normalize()
	var total int64
	if err := r.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM campaign_link_clicks WHERE campaign_id = $1`, campaignID,
	).Scan(&total); err != nil {
		return repository.Page[LinkClick]{}, err
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, campaign_id, recipient_id, team_id, url, clicked_at, user_agent, device_type
		FROM campaign_link_clicks WHERE campaign_id = $1 ORDER BY clicked_at DESC LIMIT $2 OFFSET $3
	`, campaignID, page.Limit(), page.Offset())
	if err != nil {
		return repository.Page[LinkClick]{}, err
	}
	defer rows.Close()
	items := make([]LinkClick, 0)
	for rows.Next() {
		var c LinkClick
		if err := rows.Scan(&c.ID, &c.CampaignID, &c.RecipientID, &c.TeamID, &c.URL, &c.ClickedAt, &c.UserAgent, &c.DeviceType); err != nil {
			return repository.Page[LinkClick]{}, err
		}
		items = append(items, c)
	}
	return repository.Page[LinkClick]{Items: items, Pagination: repository.NewMeta(page.Page, page.PageSize, total)}, rows.Err()
}

func scanFunnel(row pgx.Row) (*CampaignFunnel, error) { return scanFunnelRow(row) }

func scanFunnelRow(row pgx.Row) (*CampaignFunnel, error) {
	var f CampaignFunnel
	err := row.Scan(
		&f.CampaignID, &f.TeamID, &f.Name, &f.Status, &f.TotalContacts, &f.Sent, &f.Delivered,
		&f.Read, &f.Failed, &f.Replies, &f.Clicked, &f.StartedAt, &f.CompletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan campaign funnel: %w", err)
	}
	return &f, nil
}
