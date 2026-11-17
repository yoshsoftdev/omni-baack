package report

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// MaxInboxAnalyticsDays bounds one query (the per-conversation first-response lookups are not free).
const MaxInboxAnalyticsDays = 92

// InboxAnalyticsQuery selects conversations and messages in [From, To), bucketed by day in Location.
type InboxAnalyticsQuery struct {
	From, To  time.Time
	Location  *time.Location
	ChannelID *uuid.UUID
}

// InboxSummary: durations are seconds; nil when there is no data to measure.
type InboxSummary struct {
	ConversationsStarted int64    `json:"conversations_started"`
	MessagesReceived     int64    `json:"messages_received"`
	MessagesSentByAgents int64    `json:"messages_sent_by_agents"`
	FirstResponseMedian  *float64 `json:"first_response_median_seconds"`
	FirstResponseP90     *float64 `json:"first_response_p90_seconds"`
	Resolved             int64    `json:"resolved"`
	ResolutionMedian     *float64 `json:"resolution_median_seconds"`
	Unanswered           int64    `json:"unanswered"`
	OpenNow              int64    `json:"open_now"`
}

// InboxDay is one calendar day in the query's time zone.
type InboxDay struct {
	Date                 string `json:"date"`
	ConversationsStarted int64  `json:"conversations_started"`
	MessagesReceived     int64  `json:"messages_received"`
	Resolved             int64  `json:"resolved"`
}

// AgentStats is one team member's work in the range.
type AgentStats struct {
	UserID              uuid.UUID `json:"user_id"`
	Name                string    `json:"name"`
	MessagesSent        int64     `json:"messages_sent"`
	Resolved            int64     `json:"resolved"`
	FirstResponseMedian *float64  `json:"first_response_median_seconds"`
	OpenAssigned        int64     `json:"open_assigned"`
}

type InboxAnalytics struct {
	Summary InboxSummary `json:"summary"`
	Daily   []InboxDay   `json:"daily"`
	Agents  []AgentStats `json:"agents"`
}

// InboxAnalyticsStore computes the analytics (PostgresInboxAnalytics).
type InboxAnalyticsStore interface {
	InboxAnalytics(ctx context.Context, teamID uuid.UUID, q InboxAnalyticsQuery) (*InboxAnalytics, error)
}

// SetInboxAnalytics enables GetInboxAnalytics.
func (s *Service) SetInboxAnalytics(store InboxAnalyticsStore) { s.inboxAnalytics = store }

// GetInboxAnalytics reports inbox volume, response and resolution times, and per-agent work (9.3).
func (s *Service) GetInboxAnalytics(ctx context.Context, teamID, userID uuid.UUID, actorRole string, q InboxAnalyticsQuery) (*InboxAnalytics, error) {
	if err := s.authorize(actorRole, rbac.PermReportView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if err := validateDateRange(q.From, q.To); err != nil {
		return nil, err
	}
	if q.To.Sub(q.From) > MaxInboxAnalyticsDays*24*time.Hour {
		return nil, serviceerrors.NewValidation("from/to", "range must be at most 92 days")
	}
	if q.Location == nil {
		q.Location = time.UTC
	}
	if s.inboxAnalytics == nil {
		return nil, serviceerrors.NewValidation("report", "inbox analytics is not configured")
	}
	out, err := s.inboxAnalytics.InboxAnalytics(ctx, teamID, q)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	out.Daily = fillDays(out.Daily, q)
	return out, nil
}

// fillDays returns one entry per calendar day of the range (in q.Location), zero where nothing happened.
func fillDays(days []InboxDay, q InboxAnalyticsQuery) []InboxDay {
	byDate := make(map[string]InboxDay, len(days))
	for _, d := range days {
		byDate[d.Date] = d
	}
	out := []InboxDay{}
	start := q.From.In(q.Location)
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, q.Location)
	for day.Before(q.To) {
		key := day.Format("2006-01-02")
		d := byDate[key]
		d.Date = key
		out = append(out, d)
		day = day.AddDate(0, 0, 1)
	}
	return out
}

// PostgresInboxAnalytics reads conversations, messages and team members.
type PostgresInboxAnalytics struct {
	db *database.DB
}

func NewInboxAnalyticsRepository(db *database.DB) *PostgresInboxAnalytics {
	return &PostgresInboxAnalytics{db: db}
}

// Shared CTEs. $1 team, $2 from, $3 to, $4 channel (nullable).
// first_in: when the customer first wrote; conversations an agent started (no inbound before
// the first response) are excluded from first-response time.
// auto: messages sent by the platform (welcome/keyword/recontact), not by a person.
const inboxConversationsCTE = `
	conv AS (
		SELECT c.id, c.created_at, c.first_response_at, c.resolution_at, c.assigned_to, c.status,
			(SELECT MIN(m.received_at) FROM messages m
			 WHERE m.conversation_id = c.id AND m.direction = 'inbound') AS first_in
		FROM conversations c
		WHERE c.team_id = $1 AND c.deleted_at IS NULL AND ($4::uuid IS NULL OR c.channel_id = $4)
			-- Only conversations that can affect the range (first_in is a per-row lookup).
			-- shortcut: a conversation created >30 days before the range whose first inbound falls inside it
			-- is skipped for first-response time; add a first_inbound_at column if that ever matters.
			AND c.created_at < $3
			AND (c.created_at >= $2::timestamptz - INTERVAL '30 days' OR c.status = 'open' OR c.resolution_at >= $2)
	),
	auto AS (
		SELECT message_id FROM inbox_auto_replies WHERE team_id = $1 AND message_id IS NOT NULL
		UNION ALL
		SELECT message_id FROM inbox_window_recontact_sends WHERE team_id = $1 AND message_id IS NOT NULL
	)`

func (r *PostgresInboxAnalytics) InboxAnalytics(ctx context.Context, teamID uuid.UUID, q InboxAnalyticsQuery) (*InboxAnalytics, error) {
	out := &InboxAnalytics{Daily: []InboxDay{}, Agents: []AgentStats{}}
	s := &out.Summary
	err := r.db.Pool.QueryRow(ctx, `WITH `+inboxConversationsCTE+`
		SELECT
			(SELECT COUNT(*) FROM conv WHERE created_at >= $2 AND created_at < $3),
			(SELECT COUNT(*) FROM messages m WHERE m.team_id = $1 AND m.direction = 'inbound'
				AND m.received_at >= $2 AND m.received_at < $3 AND ($4::uuid IS NULL OR m.channel_id = $4)),
			(SELECT COUNT(*) FROM messages m JOIN team_members tm
				ON tm.team_id = $1 AND tm.deleted_at IS NULL AND m.sender = tm.user_id::text
			 WHERE m.team_id = $1 AND m.direction = 'outbound' AND m.created_at >= $2 AND m.created_at < $3
				AND ($4::uuid IS NULL OR m.channel_id = $4) AND m.id NOT IN (SELECT message_id FROM auto)),
			(SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM first_response_at - first_in))
			 FROM conv WHERE first_in >= $2 AND first_in < $3 AND first_response_at >= first_in),
			(SELECT percentile_cont(0.9) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM first_response_at - first_in))
			 FROM conv WHERE first_in >= $2 AND first_in < $3 AND first_response_at >= first_in),
			(SELECT COUNT(*) FROM conv WHERE resolution_at >= $2 AND resolution_at < $3),
			(SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM resolution_at - created_at))
			 FROM conv WHERE resolution_at >= $2 AND resolution_at < $3),
			(SELECT COUNT(*) FROM conv WHERE first_in >= $2 AND first_in < $3 AND first_response_at IS NULL),
			(SELECT COUNT(*) FROM conv WHERE status = 'open')
	`, teamID, q.From, q.To, q.ChannelID).Scan(
		&s.ConversationsStarted, &s.MessagesReceived, &s.MessagesSentByAgents,
		&s.FirstResponseMedian, &s.FirstResponseP90, &s.Resolved, &s.ResolutionMedian,
		&s.Unanswered, &s.OpenNow,
	)
	if err != nil {
		return nil, fmt.Errorf("inbox summary: %w", err)
	}

	// Daily buckets in the viewer's zone; missing days are filled in by the service.
	rows, err := r.db.Pool.Query(ctx, `WITH `+inboxConversationsCTE+`,
		started AS (SELECT (created_at AT TIME ZONE $5)::date AS d, COUNT(*) AS n FROM conv
			WHERE created_at >= $2 AND created_at < $3 GROUP BY 1),
		received AS (SELECT (received_at AT TIME ZONE $5)::date AS d, COUNT(*) AS n FROM messages m
			WHERE m.team_id = $1 AND m.direction = 'inbound' AND m.received_at >= $2 AND m.received_at < $3
				AND ($4::uuid IS NULL OR m.channel_id = $4) GROUP BY 1),
		resolved AS (SELECT (resolution_at AT TIME ZONE $5)::date AS d, COUNT(*) AS n FROM conv
			WHERE resolution_at >= $2 AND resolution_at < $3 GROUP BY 1),
		days AS (SELECT d FROM started UNION SELECT d FROM received UNION SELECT d FROM resolved)
		SELECT to_char(days.d, 'YYYY-MM-DD'), COALESCE(started.n, 0), COALESCE(received.n, 0), COALESCE(resolved.n, 0)
		FROM days
		LEFT JOIN started ON started.d = days.d
		LEFT JOIN received ON received.d = days.d
		LEFT JOIN resolved ON resolved.d = days.d
		ORDER BY days.d
	`, teamID, q.From, q.To, q.ChannelID, q.Location.String())
	if err != nil {
		return nil, fmt.Errorf("inbox daily: %w", err)
	}
	for rows.Next() {
		var d InboxDay
		if err := rows.Scan(&d.Date, &d.ConversationsStarted, &d.MessagesReceived, &d.Resolved); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan inbox day: %w", err)
		}
		out.Daily = append(out.Daily, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = r.db.Pool.Query(ctx, `WITH `+inboxConversationsCTE+`
		SELECT tm.user_id, u.name,
			(SELECT COUNT(*) FROM messages m
			 WHERE m.team_id = $1 AND m.direction = 'outbound' AND m.sender = tm.user_id::text
				AND m.created_at >= $2 AND m.created_at < $3 AND ($4::uuid IS NULL OR m.channel_id = $4)
				AND m.id NOT IN (SELECT message_id FROM auto)),
			(SELECT COUNT(*) FROM conv WHERE assigned_to = tm.user_id AND resolution_at >= $2 AND resolution_at < $3),
			(SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM first_response_at - first_in))
			 FROM conv WHERE assigned_to = tm.user_id AND first_in >= $2 AND first_in < $3 AND first_response_at >= first_in),
			(SELECT COUNT(*) FROM conv WHERE assigned_to = tm.user_id AND status = 'open')
		FROM team_members tm
		JOIN users u ON u.id = tm.user_id
		WHERE tm.team_id = $1 AND tm.deleted_at IS NULL
		ORDER BY u.name
	`, teamID, q.From, q.To, q.ChannelID)
	if err != nil {
		return nil, fmt.Errorf("inbox agents: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var a AgentStats
		if err := rows.Scan(&a.UserID, &a.Name, &a.MessagesSent, &a.Resolved, &a.FirstResponseMedian, &a.OpenAssigned); err != nil {
			return nil, fmt.Errorf("scan agent stats: %w", err)
		}
		out.Agents = append(out.Agents, a)
	}
	return out, rows.Err()
}
