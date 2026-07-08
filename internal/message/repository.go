package message

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	sharedwamid "github.com/omnichannel/omni-chan-v2/shared/wamid"
)

// Repository (MessageRepository) persists inbox messages (partitioned by created_at).
type Repository interface {
	Create(ctx context.Context, m *Message) error
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*Message, error)
	FindByWAMID(ctx context.Context, wamid string) (*Message, error)
	FindByWAMIDKey(ctx context.Context, teamID, conversationID uuid.UUID, key string) (*Message, error)
	ListByConversation(ctx context.Context, teamID, conversationID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Message], error)
	UpdateStatus(ctx context.Context, teamID, id uuid.UUID, status Status) error
	MarkDeletedByID(ctx context.Context, teamID, id uuid.UUID, source DeletedBySource) error
	MarkDeletedByWAMID(ctx context.Context, teamID uuid.UUID, wamid string, source DeletedBySource) error
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, m *Message) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO messages (
			id, conversation_id, team_id, channel_id, direction, sender, recipient,
			content, content_type, wamid, wamid_key, reply_to_wamid, reply_to_message_id, status, is_deleted, deleted_at, deleted_by_source, raw_payload, received_at, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
	`, m.ID, m.ConversationID, m.TeamID, m.ChannelID, string(m.Direction), m.Sender, m.Recipient,
		m.Content, string(m.ContentType), m.WAMID, wamidKeyPtr(m.WAMID), m.ReplyToWAMID, m.ReplyToMessageID, statusPtr(m.Status), m.IsDeleted, m.DeletedAt, deletedBySourcePtr(m.DeletedBySource), nullableJSON(m.RawPayload), m.ReceivedAt, m.CreatedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, id uuid.UUID) (*Message, error) {
	return scanMessage(r.db.Pool.QueryRow(ctx, `
		SELECT id, conversation_id, team_id, channel_id, direction, sender, recipient,
			content, content_type, wamid, reply_to_wamid, reply_to_message_id, status, is_deleted, deleted_at, deleted_by_source, raw_payload, received_at, created_at
		FROM messages WHERE id = $1 AND team_id = $2
		ORDER BY created_at DESC LIMIT 1
	`, id, teamID))
}

func (r *PostgresRepository) FindByWAMID(ctx context.Context, wamid string) (*Message, error) {
	return scanMessage(r.db.Pool.QueryRow(ctx, `
		SELECT id, conversation_id, team_id, channel_id, direction, sender, recipient,
			content, content_type, wamid, reply_to_wamid, reply_to_message_id, status, is_deleted, deleted_at, deleted_by_source, raw_payload, received_at, created_at
		FROM messages WHERE wamid = $1
		ORDER BY created_at DESC LIMIT 1
	`, wamid))
}

func (r *PostgresRepository) FindByWAMIDKey(ctx context.Context, teamID, conversationID uuid.UUID, key string) (*Message, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, repository.ErrNotFound
	}

	msg, err := scanMessage(r.db.Pool.QueryRow(ctx, `
		SELECT id, conversation_id, team_id, channel_id, direction, sender, recipient,
			content, content_type, wamid, reply_to_wamid, reply_to_message_id, status, is_deleted, deleted_at, deleted_by_source, raw_payload, received_at, created_at
		FROM messages
		WHERE team_id = $1 AND conversation_id = $2 AND wamid_key = $3
		ORDER BY created_at DESC LIMIT 1
	`, teamID, conversationID, key))
	if err == nil {
		return msg, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, conversation_id, team_id, channel_id, direction, sender, recipient,
			content, content_type, wamid, reply_to_wamid, reply_to_message_id, status, is_deleted, deleted_at, deleted_by_source, raw_payload, received_at, created_at
		FROM messages
		WHERE team_id = $1 AND conversation_id = $2 AND wamid IS NOT NULL
		ORDER BY created_at DESC LIMIT 200
	`, teamID, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		candidate, scanErr := scanMessageRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		if candidate.WAMID != nil && sharedwamid.ExtractMessageKey(*candidate.WAMID) == key {
			return candidate, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (r *PostgresRepository) ListByConversation(ctx context.Context, teamID, conversationID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Message], error) {
	page = page.Normalize()
	args := []any{teamID, conversationID}
	where := `team_id = $1 AND conversation_id = $2`
	if filter.Direction != nil {
		args = append(args, string(*filter.Direction))
		where += fmt.Sprintf(` AND direction = $%d`, len(args))
	}
	if filter.Since != nil {
		args = append(args, *filter.Since)
		where += fmt.Sprintf(` AND created_at >= $%d`, len(args))
	}
	var total int64
	if err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM messages WHERE `+where, args...).Scan(&total); err != nil {
		return repository.Page[Message]{}, err
	}
	args = append(args, page.Limit(), page.Offset())
	rows, err := r.db.Pool.Query(ctx, fmt.Sprintf(`
		SELECT id, conversation_id, team_id, channel_id, direction, sender, recipient,
			content, content_type, wamid, reply_to_wamid, reply_to_message_id, status, is_deleted, deleted_at, deleted_by_source, raw_payload, received_at, created_at
		FROM messages WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d
	`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return repository.Page[Message]{}, err
	}
	defer rows.Close()
	items := make([]Message, 0)
	for rows.Next() {
		m, err := scanMessageRow(rows)
		if err != nil {
			return repository.Page[Message]{}, err
		}
		items = append(items, *m)
	}
	return repository.Page[Message]{Items: items, Pagination: repository.NewMeta(page.Page, page.PageSize, total)}, rows.Err()
}

func (r *PostgresRepository) UpdateStatus(ctx context.Context, teamID, id uuid.UUID, status Status) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE messages SET status = $1
		WHERE id = $2 AND team_id = $3 AND created_at = (
			SELECT created_at FROM messages WHERE id = $2 AND team_id = $3 ORDER BY created_at DESC LIMIT 1
		)
	`, string(status), id, teamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) MarkDeletedByID(ctx context.Context, teamID, id uuid.UUID, source DeletedBySource) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE messages
		SET is_deleted = TRUE, deleted_at = NOW(), deleted_by_source = $1
		WHERE id = $2 AND team_id = $3
	`, string(source), id, teamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) MarkDeletedByWAMID(ctx context.Context, teamID uuid.UUID, wamid string, source DeletedBySource) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE messages
		SET is_deleted = TRUE, deleted_at = NOW(), deleted_by_source = $1
		WHERE team_id = $2 AND wamid = $3
	`, string(source), teamID, wamid)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func statusPtr(s *Status) *string {
	if s == nil {
		return nil
	}
	v := string(*s)
	return &v
}

func deletedBySourcePtr(s *DeletedBySource) *string {
	if s == nil {
		return nil
	}
	v := string(*s)
	return &v
}

func nullableJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

func wamidKeyPtr(wamid *string) *string {
	if wamid == nil {
		return nil
	}
	key := sharedwamid.ExtractMessageKey(*wamid)
	if key == "" {
		return nil
	}
	return &key
}

func scanMessage(row pgx.Row) (*Message, error) { return scanMessageRow(row) }

func scanMessageRow(row pgx.Row) (*Message, error) {
	var m Message
	var direction, contentType string
	var status *string
	var deletedBySource *string
	err := row.Scan(
		&m.ID, &m.ConversationID, &m.TeamID, &m.ChannelID, &direction, &m.Sender, &m.Recipient,
		&m.Content, &contentType, &m.WAMID, &m.ReplyToWAMID, &m.ReplyToMessageID, &status, &m.IsDeleted, &m.DeletedAt, &deletedBySource, &m.RawPayload, &m.ReceivedAt, &m.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan message: %w", err)
	}
	m.Direction = Direction(direction)
	m.ContentType = ContentType(contentType)
	if status != nil {
		s := Status(*status)
		m.Status = &s
	}
	if deletedBySource != nil {
		v := DeletedBySource(*deletedBySource)
		m.DeletedBySource = &v
	}
	return &m, nil
}
