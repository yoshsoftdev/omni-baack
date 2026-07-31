package inbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

const (
	// MaxWindowRecontactAfterHours is the latest auto-send delay before the Meta 24h window closes.
	MaxWindowRecontactAfterHours = 23
	MinWindowRecontactAfterHours = 1
	DefaultWindowRecontactAfterHours = 23
	DefaultWindowRecontactText = "Halo, apakah masih ada yang bisa kami bantu? Silakan balas pesan ini agar kami dapat terus membantu Anda."
)

// WindowRecontactSettings is the team-level auto re-contact config for the Meta free-text window.
type WindowRecontactSettings struct {
	Enabled    bool   `json:"enabled"`
	Text       string `json:"text"`
	AfterHours int    `json:"after_hours"`
}

func DefaultWindowRecontactSettings() WindowRecontactSettings {
	return WindowRecontactSettings{
		Enabled:    false,
		Text:       DefaultWindowRecontactText,
		AfterHours: DefaultWindowRecontactAfterHours,
	}
}

func NormalizeWindowRecontactSettings(in WindowRecontactSettings) (WindowRecontactSettings, error) {
	out := in
	out.Text = strings.TrimSpace(out.Text)
	if out.AfterHours < MinWindowRecontactAfterHours || out.AfterHours > MaxWindowRecontactAfterHours {
		return WindowRecontactSettings{}, serviceerrors.NewValidation(
			"after_hours",
			fmt.Sprintf("must be between %d and %d", MinWindowRecontactAfterHours, MaxWindowRecontactAfterHours),
		)
	}
	if out.Enabled && out.Text == "" {
		return WindowRecontactSettings{}, serviceerrors.NewValidation("text", "is required when enabled")
	}
	if out.Text == "" {
		out.Text = DefaultWindowRecontactText
	}
	return out, nil
}

type WindowRecontactCandidate struct {
	TeamID                uuid.UUID
	ConversationID        uuid.UUID
	ContactID             uuid.UUID
	ChannelID             *uuid.UUID
	ChannelType           string
	LastCustomerMessageAt time.Time
	Text                  string
	AfterHours            int
}

type WindowRecontactSend struct {
	ID                 uuid.UUID
	TeamID             uuid.UUID
	ConversationID     uuid.UUID
	CustomerMessageAt  time.Time
	MessageID          *uuid.UUID
	SentAt             time.Time
}

// WindowRecontactStore persists settings, due candidates, and send ledger rows.
type WindowRecontactStore interface {
	GetWindowRecontactSettings(ctx context.Context, teamID uuid.UUID) (WindowRecontactSettings, error)
	UpsertWindowRecontactSettings(ctx context.Context, teamID uuid.UUID, settings WindowRecontactSettings) (WindowRecontactSettings, error)
	ListDueWindowRecontacts(ctx context.Context, limit int, now time.Time) ([]WindowRecontactCandidate, error)
	TryClaimWindowRecontact(ctx context.Context, teamID, conversationID uuid.UUID, customerMessageAt time.Time) (claimed bool, sendID uuid.UUID, err error)
	MarkWindowRecontactSent(ctx context.Context, sendID uuid.UUID, messageID uuid.UUID) error
	DeleteWindowRecontactClaim(ctx context.Context, sendID uuid.UUID) error
	FindTeamOwnerUserID(ctx context.Context, teamID uuid.UUID) (uuid.UUID, error)
}

func (r *PostgresSettingsRepository) GetWindowRecontactSettings(ctx context.Context, teamID uuid.UUID) (WindowRecontactSettings, error) {
	defaults := DefaultWindowRecontactSettings()
	var enabled bool
	var text string
	var afterHours int
	err := r.db.Pool.QueryRow(ctx, `
		SELECT window_recontact_enabled, window_recontact_text, window_recontact_after_hours
		FROM team_inbox_settings WHERE team_id = $1
	`, teamID).Scan(&enabled, &text, &afterHours)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return defaults, nil
		}
		return WindowRecontactSettings{}, fmt.Errorf("get window recontact settings: %w", err)
	}
	if strings.TrimSpace(text) == "" {
		text = defaults.Text
	}
	if afterHours < MinWindowRecontactAfterHours || afterHours > MaxWindowRecontactAfterHours {
		afterHours = defaults.AfterHours
	}
	return WindowRecontactSettings{Enabled: enabled, Text: text, AfterHours: afterHours}, nil
}

func (r *PostgresSettingsRepository) UpsertWindowRecontactSettings(
	ctx context.Context,
	teamID uuid.UUID,
	settings WindowRecontactSettings,
) (WindowRecontactSettings, error) {
	normalized, err := NormalizeWindowRecontactSettings(settings)
	if err != nil {
		return WindowRecontactSettings{}, err
	}
	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO team_inbox_settings (
			team_id, window_recontact_enabled, window_recontact_text, window_recontact_after_hours, updated_at
		) VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (team_id) DO UPDATE SET
			window_recontact_enabled = EXCLUDED.window_recontact_enabled,
			window_recontact_text = EXCLUDED.window_recontact_text,
			window_recontact_after_hours = EXCLUDED.window_recontact_after_hours,
			updated_at = NOW()
	`, teamID, normalized.Enabled, normalized.Text, normalized.AfterHours)
	if err != nil {
		return WindowRecontactSettings{}, repository.MapError(err)
	}
	return normalized, nil
}

func (r *PostgresSettingsRepository) ListDueWindowRecontacts(
	ctx context.Context,
	limit int,
	now time.Time,
) ([]WindowRecontactCandidate, error) {
	if limit < 1 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT
			c.team_id,
			c.id,
			c.contact_id,
			c.channel_id,
			c.channel_type::text,
			c.last_customer_message_at,
			s.window_recontact_text,
			s.window_recontact_after_hours
		FROM conversations c
		INNER JOIN team_inbox_settings s ON s.team_id = c.team_id
		WHERE s.window_recontact_enabled = TRUE
		  AND LENGTH(TRIM(s.window_recontact_text)) > 0
		  AND c.deleted_at IS NULL
		  AND c.status IN ('open', 'pending')
		  AND c.channel_type::text IN ('whatsapp', 'instagram')
		  AND c.last_customer_message_at IS NOT NULL
		  AND c.last_customer_message_at <= ($1::timestamptz - (s.window_recontact_after_hours * INTERVAL '1 hour'))
		  AND c.last_customer_message_at > ($1::timestamptz - INTERVAL '24 hours')
		  AND (c.last_agent_message_at IS NULL OR c.last_agent_message_at < c.last_customer_message_at)
		  AND NOT EXISTS (
			SELECT 1
			FROM inbox_window_recontact_sends r
			WHERE r.conversation_id = c.id
			  AND r.customer_message_at = c.last_customer_message_at
		  )
		ORDER BY c.last_customer_message_at ASC
		LIMIT $2
	`, now.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("list due window recontacts: %w", err)
	}
	defer rows.Close()

	out := make([]WindowRecontactCandidate, 0, limit)
	for rows.Next() {
		var item WindowRecontactCandidate
		if scanErr := rows.Scan(
			&item.TeamID,
			&item.ConversationID,
			&item.ContactID,
			&item.ChannelID,
			&item.ChannelType,
			&item.LastCustomerMessageAt,
			&item.Text,
			&item.AfterHours,
		); scanErr != nil {
			return nil, fmt.Errorf("scan window recontact candidate: %w", scanErr)
		}
		item.Text = strings.TrimSpace(item.Text)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresSettingsRepository) TryClaimWindowRecontact(
	ctx context.Context,
	teamID, conversationID uuid.UUID,
	customerMessageAt time.Time,
) (bool, uuid.UUID, error) {
	sendID := uuid.New()
	tag, err := r.db.Pool.Exec(ctx, `
		INSERT INTO inbox_window_recontact_sends (
			id, team_id, conversation_id, customer_message_at, sent_at
		) VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (conversation_id, customer_message_at) DO NOTHING
	`, sendID, teamID, conversationID, customerMessageAt.UTC())
	if err != nil {
		return false, uuid.Nil, repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return false, uuid.Nil, nil
	}
	return true, sendID, nil
}

func (r *PostgresSettingsRepository) MarkWindowRecontactSent(ctx context.Context, sendID, messageID uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE inbox_window_recontact_sends
		SET message_id = $2, sent_at = NOW()
		WHERE id = $1
	`, sendID, messageID)
	return repository.MapError(err)
}

func (r *PostgresSettingsRepository) DeleteWindowRecontactClaim(ctx context.Context, sendID uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `
		DELETE FROM inbox_window_recontact_sends WHERE id = $1 AND message_id IS NULL
	`, sendID)
	return repository.MapError(err)
}

func (r *PostgresSettingsRepository) FindTeamOwnerUserID(ctx context.Context, teamID uuid.UUID) (uuid.UUID, error) {
	var userID uuid.UUID
	err := r.db.Pool.QueryRow(ctx, `
		SELECT user_id
		FROM team_members
		WHERE team_id = $1 AND role = 'owner' AND deleted_at IS NULL
		ORDER BY created_at ASC
		LIMIT 1
	`, teamID).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, repository.ErrNotFound
		}
		return uuid.Nil, fmt.Errorf("find team owner: %w", err)
	}
	return userID, nil
}

// Ensure PostgresSettingsRepository implements WindowRecontactStore.
var _ WindowRecontactStore = (*PostgresSettingsRepository)(nil)
