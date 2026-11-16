package inbox

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// AutoReplyStore persists welcome settings, keyword rules and the send ledger.
type AutoReplyStore interface {
	GetWelcomeSettings(ctx context.Context, teamID uuid.UUID) (WelcomeSettings, error)
	UpsertWelcomeSettings(ctx context.Context, teamID uuid.UUID, settings WelcomeSettings) error
	ListKeywordRules(ctx context.Context, teamID uuid.UUID) ([]KeywordRule, error)
	CountKeywordRules(ctx context.Context, teamID uuid.UUID) (int, error)
	CreateKeywordRule(ctx context.Context, teamID uuid.UUID, rule *KeywordRule) error
	UpdateKeywordRule(ctx context.Context, teamID uuid.UUID, rule *KeywordRule) error
	DeleteKeywordRule(ctx context.Context, teamID, id uuid.UUID) error
	TryClaimAutoReply(ctx context.Context, teamID, conversationID, inboundMessageID uuid.UUID, kind string, ruleID *uuid.UUID) (claimed bool, claimID uuid.UUID, err error)
	MarkAutoReplySent(ctx context.Context, claimID, messageID uuid.UUID) error
	DeleteAutoReplyClaim(ctx context.Context, claimID uuid.UUID) error
	FindTeamOwnerUserID(ctx context.Context, teamID uuid.UUID) (uuid.UUID, error)
}

func (r *PostgresSettingsRepository) GetWelcomeSettings(ctx context.Context, teamID uuid.UUID) (WelcomeSettings, error) {
	var out WelcomeSettings
	err := r.db.Pool.QueryRow(ctx, `
		SELECT welcome_enabled, welcome_text FROM team_inbox_settings WHERE team_id = $1
	`, teamID).Scan(&out.Enabled, &out.Text)
	if errors.Is(err, pgx.ErrNoRows) {
		return WelcomeSettings{}, nil
	}
	if err != nil {
		return WelcomeSettings{}, fmt.Errorf("get welcome settings: %w", err)
	}
	return out, nil
}

func (r *PostgresSettingsRepository) UpsertWelcomeSettings(ctx context.Context, teamID uuid.UUID, settings WelcomeSettings) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO team_inbox_settings (team_id, welcome_enabled, welcome_text, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (team_id) DO UPDATE SET
			welcome_enabled = EXCLUDED.welcome_enabled,
			welcome_text = EXCLUDED.welcome_text,
			updated_at = NOW()
	`, teamID, settings.Enabled, settings.Text)
	return repository.MapError(err)
}

const keywordRuleColumns = `id, keywords, match_type, reply_text, enabled, sort_order, created_at, updated_at`

// ListKeywordRules returns rules in evaluation order (sort_order, then creation).
func (r *PostgresSettingsRepository) ListKeywordRules(ctx context.Context, teamID uuid.UUID) ([]KeywordRule, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT `+keywordRuleColumns+` FROM inbox_keyword_rules
		WHERE team_id = $1 ORDER BY sort_order, created_at
	`, teamID)
	if err != nil {
		return nil, fmt.Errorf("list keyword rules: %w", err)
	}
	defer rows.Close()
	out := []KeywordRule{}
	for rows.Next() {
		var rule KeywordRule
		if err := rows.Scan(&rule.ID, &rule.Keywords, &rule.MatchType, &rule.ReplyText, &rule.Enabled, &rule.SortOrder, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan keyword rule: %w", err)
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

func (r *PostgresSettingsRepository) CountKeywordRules(ctx context.Context, teamID uuid.UUID) (int, error) {
	var n int
	err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM inbox_keyword_rules WHERE team_id = $1`, teamID).Scan(&n)
	return n, repository.MapError(err)
}

func (r *PostgresSettingsRepository) CreateKeywordRule(ctx context.Context, teamID uuid.UUID, rule *KeywordRule) error {
	return repository.MapError(r.db.Pool.QueryRow(ctx, `
		INSERT INTO inbox_keyword_rules (id, team_id, keywords, match_type, reply_text, enabled, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at
	`, rule.ID, teamID, rule.Keywords, rule.MatchType, rule.ReplyText, rule.Enabled, rule.SortOrder).
		Scan(&rule.CreatedAt, &rule.UpdatedAt))
}

func (r *PostgresSettingsRepository) UpdateKeywordRule(ctx context.Context, teamID uuid.UUID, rule *KeywordRule) error {
	err := r.db.Pool.QueryRow(ctx, `
		UPDATE inbox_keyword_rules
		SET keywords = $3, match_type = $4, reply_text = $5, enabled = $6, sort_order = $7, updated_at = NOW()
		WHERE team_id = $1 AND id = $2
		RETURNING created_at, updated_at
	`, teamID, rule.ID, rule.Keywords, rule.MatchType, rule.ReplyText, rule.Enabled, rule.SortOrder).
		Scan(&rule.CreatedAt, &rule.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return repository.ErrNotFound
	}
	return repository.MapError(err)
}

func (r *PostgresSettingsRepository) DeleteKeywordRule(ctx context.Context, teamID, id uuid.UUID) error {
	tag, err := r.db.Pool.Exec(ctx, `DELETE FROM inbox_keyword_rules WHERE team_id = $1 AND id = $2`, teamID, id)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// TryClaimAutoReply reserves the (inbound message, kind) slot; false means it was already sent or is being sent.
func (r *PostgresSettingsRepository) TryClaimAutoReply(
	ctx context.Context,
	teamID, conversationID, inboundMessageID uuid.UUID,
	kind string,
	ruleID *uuid.UUID,
) (bool, uuid.UUID, error) {
	claimID := uuid.New()
	tag, err := r.db.Pool.Exec(ctx, `
		INSERT INTO inbox_auto_replies (id, team_id, conversation_id, inbound_message_id, kind, rule_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (inbound_message_id, kind) DO NOTHING
	`, claimID, teamID, conversationID, inboundMessageID, kind, ruleID)
	if err != nil {
		return false, uuid.Nil, repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return false, uuid.Nil, nil
	}
	return true, claimID, nil
}

func (r *PostgresSettingsRepository) MarkAutoReplySent(ctx context.Context, claimID, messageID uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `UPDATE inbox_auto_replies SET message_id = $2, sent_at = NOW() WHERE id = $1`, claimID, messageID)
	return repository.MapError(err)
}

// DeleteAutoReplyClaim releases a claim whose send failed, so a later redelivery may retry.
func (r *PostgresSettingsRepository) DeleteAutoReplyClaim(ctx context.Context, claimID uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM inbox_auto_replies WHERE id = $1 AND message_id IS NULL`, claimID)
	return repository.MapError(err)
}
