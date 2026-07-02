package conversation

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type ActivityRepository interface {
	Create(ctx context.Context, activity *Activity) error
	ListByConversation(ctx context.Context, teamID, conversationID uuid.UUID, limit int) ([]Activity, error)
}

type PostgresActivityRepository struct {
	db *database.DB
}

func NewActivityRepository(db *database.DB) *PostgresActivityRepository {
	return &PostgresActivityRepository{db: db}
}

func (r *PostgresActivityRepository) Create(ctx context.Context, activity *Activity) error {
	meta := activity.Metadata
	if meta == nil {
		meta = json.RawMessage(`{}`)
	}
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO conversation_activities (
			id, team_id, conversation_id, activity_type, actor_id, description, metadata, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, activity.ID, activity.TeamID, activity.ConversationID, string(activity.Type),
		activity.ActorID, activity.Description, meta, activity.CreatedAt)
	return repository.MapError(err)
}

func (r *PostgresActivityRepository) ListByConversation(ctx context.Context, teamID, conversationID uuid.UUID, limit int) ([]Activity, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT
			a.id, a.team_id, a.conversation_id, a.activity_type, a.actor_id,
			COALESCE(u.name, ''), a.description, a.metadata, a.created_at
		FROM conversation_activities a
		LEFT JOIN users u ON u.id = a.actor_id
		WHERE a.team_id = $1 AND a.conversation_id = $2
		ORDER BY a.created_at DESC
		LIMIT $3
	`, teamID, conversationID, limit)
	if err != nil {
		return nil, repository.MapError(err)
	}
	defer rows.Close()

	items := make([]Activity, 0)
	for rows.Next() {
		var item Activity
		var meta []byte
		if err := rows.Scan(
			&item.ID, &item.TeamID, &item.ConversationID, &item.Type, &item.ActorID,
			&item.ActorName, &item.Description, &meta, &item.CreatedAt,
		); err != nil {
			return nil, repository.MapError(err)
		}
		if len(meta) > 0 {
			item.Metadata = json.RawMessage(meta)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type NoteRepository interface {
	Create(ctx context.Context, note *Note) error
	ListByConversation(ctx context.Context, teamID, conversationID uuid.UUID) ([]Note, error)
	SoftDelete(ctx context.Context, teamID, noteID uuid.UUID) error
}

type PostgresNoteRepository struct {
	db *database.DB
}

func NewNoteRepository(db *database.DB) *PostgresNoteRepository {
	return &PostgresNoteRepository{db: db}
}

func (r *PostgresNoteRepository) Create(ctx context.Context, note *Note) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO conversation_notes (
			id, team_id, conversation_id, author_id, content, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, note.ID, note.TeamID, note.ConversationID, note.AuthorID, note.Content, note.CreatedAt, note.UpdatedAt)
	return repository.MapError(err)
}

func (r *PostgresNoteRepository) ListByConversation(ctx context.Context, teamID, conversationID uuid.UUID) ([]Note, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT
			n.id, n.team_id, n.conversation_id, n.author_id,
			COALESCE(u.name, ''), n.content, n.created_at, n.updated_at, n.deleted_at
		FROM conversation_notes n
		INNER JOIN users u ON u.id = n.author_id
		WHERE n.team_id = $1 AND n.conversation_id = $2 AND n.deleted_at IS NULL
		ORDER BY n.created_at DESC
	`, teamID, conversationID)
	if err != nil {
		return nil, repository.MapError(err)
	}
	defer rows.Close()

	items := make([]Note, 0)
	for rows.Next() {
		var item Note
		if err := rows.Scan(
			&item.ID, &item.TeamID, &item.ConversationID, &item.AuthorID,
			&item.AuthorName, &item.Content, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
		); err != nil {
			return nil, repository.MapError(err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresNoteRepository) SoftDelete(ctx context.Context, teamID, noteID uuid.UUID) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE conversation_notes
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, noteID, teamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

var _ ActivityRepository = (*PostgresActivityRepository)(nil)
var _ NoteRepository = (*PostgresNoteRepository)(nil)
