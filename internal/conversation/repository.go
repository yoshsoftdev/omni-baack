package conversation

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// Repository (ConversationRepository) persists inbox conversations.
type Repository interface {
	Create(ctx context.Context, c *Conversation) error
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*Conversation, error)
	FindByContactChannel(ctx context.Context, teamID, contactID uuid.UUID, channelID *uuid.UUID) (*Conversation, error)
	List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Conversation], error)
	UpdateStatus(ctx context.Context, teamID, id uuid.UUID, status Status) error
	UpdateAssignment(ctx context.Context, teamID, id uuid.UUID, assignedTo *uuid.UUID) error
	UpdateUnreadCount(ctx context.Context, teamID, id uuid.UUID, count int) error
	UpdateLastMessage(ctx context.Context, teamID, id uuid.UUID, update LastMessageUpdate) error
	ApplyInboxSync(ctx context.Context, teamID, id uuid.UUID, update InboxSyncUpdate) error
	MarkRead(ctx context.Context, teamID, id uuid.UUID) error
	CountOpenByAssignee(ctx context.Context, teamID, userID uuid.UUID) (int64, error)
	CountByTeamFilter(ctx context.Context, teamID uuid.UUID, filter ListFilter) (int64, error)
	SoftDelete(ctx context.Context, teamID, id uuid.UUID) error
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, c *Conversation) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO conversations (
			id, team_id, contact_id, channel_id, channel_type, status, assigned_to,
			unread_count, last_message_preview, last_message_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, c.ID, c.TeamID, c.ContactID, c.ChannelID, string(c.ChannelType), string(c.Status),
		c.AssignedTo, c.UnreadCount, c.LastMessagePreview, c.LastMessageAt, c.CreatedAt, c.UpdatedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, id uuid.UUID) (*Conversation, error) {
	return scanConversation(r.db.Pool.QueryRow(ctx, `
		SELECT `+conversationSelectCols+`
		FROM conversations WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL
	`, id, teamID))
}

func (r *PostgresRepository) FindByContactChannel(ctx context.Context, teamID, contactID uuid.UUID, channelID *uuid.UUID) (*Conversation, error) {
	return scanConversation(r.db.Pool.QueryRow(ctx, `
		SELECT `+conversationSelectCols+`
		FROM conversations
		WHERE team_id = $1 AND contact_id = $2 AND channel_id IS NOT DISTINCT FROM $3 AND deleted_at IS NULL
		LIMIT 1
	`, teamID, contactID, channelID))
}

func (r *PostgresRepository) List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Conversation], error) {
	page = page.Normalize()
	args := []any{teamID}
	where := `team_id = $1 AND deleted_at IS NULL`
	if filter.Status != nil {
		args = append(args, string(*filter.Status))
		where += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	if filter.AssignedTo != nil {
		args = append(args, *filter.AssignedTo)
		where += fmt.Sprintf(` AND assigned_to = $%d`, len(args))
	}
	if filter.ChannelID != nil {
		args = append(args, *filter.ChannelID)
		where += fmt.Sprintf(` AND channel_id = $%d`, len(args))
	}
	switch filter.View {
	case ViewMine:
		if filter.ActorID != nil {
			args = append(args, *filter.ActorID)
			where += fmt.Sprintf(` AND assigned_to = $%d`, len(args))
		}
	case ViewUnassigned:
		where += ` AND assigned_to IS NULL`
	case ViewUnread:
		where += ` AND unread_count > 0`
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		where += fmt.Sprintf(` AND last_message_preview ILIKE $%d`, len(args))
	}
	var total int64
	if err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM conversations WHERE `+where, args...).Scan(&total); err != nil {
		return repository.Page[Conversation]{}, err
	}
	args = append(args, page.Limit(), page.Offset())
	rows, err := r.db.Pool.Query(ctx, fmt.Sprintf(`
		SELECT `+conversationSelectCols+`
		FROM conversations WHERE %s ORDER BY last_message_at DESC NULLS LAST, created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return repository.Page[Conversation]{}, err
	}
	defer rows.Close()
	items := make([]Conversation, 0)
	for rows.Next() {
		c, err := scanConversationRow(rows)
		if err != nil {
			return repository.Page[Conversation]{}, err
		}
		items = append(items, *c)
	}
	return repository.Page[Conversation]{Items: items, Pagination: repository.NewMeta(page.Page, page.PageSize, total)}, rows.Err()
}

func (r *PostgresRepository) UpdateStatus(ctx context.Context, teamID, id uuid.UUID, status Status) error {
	resolution := ""
	if status == StatusResolved || status == StatusClosed {
		resolution = ", resolution_at = COALESCE(resolution_at, NOW())"
	}
	tag, err := r.db.Pool.Exec(ctx,
		`UPDATE conversations SET status = $1, updated_at = NOW()`+resolution+` WHERE id = $2 AND team_id = $3 AND deleted_at IS NULL`,
		string(status), id, teamID,
	)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) UpdateAssignment(ctx context.Context, teamID, id uuid.UUID, assignedTo *uuid.UUID) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE conversations SET assigned_to = $1, updated_at = NOW()
		WHERE id = $2 AND team_id = $3 AND deleted_at IS NULL
	`, assignedTo, id, teamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) UpdateUnreadCount(ctx context.Context, teamID, id uuid.UUID, count int) error {
	tag, err := r.db.Pool.Exec(ctx,
		`UPDATE conversations SET unread_count = $1, updated_at = NOW() WHERE id = $2 AND team_id = $3 AND deleted_at IS NULL`,
		count, id, teamID,
	)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) UpdateLastMessage(ctx context.Context, teamID, id uuid.UUID, update LastMessageUpdate) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE conversations SET last_message_preview = $1, last_message_at = $2, updated_at = NOW()
		WHERE id = $3 AND team_id = $4 AND deleted_at IS NULL
	`, update.Preview, update.At, id, teamID)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ApplyInboxSync(ctx context.Context, teamID, id uuid.UUID, u InboxSyncUpdate) error {
	unread := u.UnreadCount
	if unread < 0 {
		unread = 0
	}
	status := string(u.Status)
	if status == "" {
		status = string(StatusOpen)
	}
	query := `
		UPDATE conversations SET
			status = $1,
			unread_count = $2,
			last_message_preview = CASE WHEN $3 <> '' THEN $3 ELSE last_message_preview END,
			last_message_at = CASE WHEN $12 THEN $4 ELSE last_message_at END,
			last_customer_message_at = COALESCE($5, last_customer_message_at),
			last_agent_message_at = COALESCE($6, last_agent_message_at),
			first_response_at = CASE WHEN $7 THEN COALESCE(first_response_at, NOW()) ELSE first_response_at END,
			resolution_at = CASE
				WHEN $8 THEN NULL
				WHEN $9 THEN NOW()
				ELSE resolution_at
			END,
			updated_at = NOW()
		WHERE id = $11 AND team_id = $12 AND deleted_at IS NULL
	`
	tag, err := r.db.Pool.Exec(ctx, query,
		status, unread, u.LastMessagePreview, u.LastMessageAt,
		u.LastCustomerMessageAt, u.LastAgentMessageAt,
		u.SetFirstResponseAt, u.ClearResolutionAt, u.SetResolutionAt,
		u.TouchLastMessage,
		id, teamID,
	)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) MarkRead(ctx context.Context, teamID, id uuid.UUID) error {
	tag, err := r.db.Pool.Exec(ctx,
		`UPDATE conversations SET unread_count = 0, updated_at = NOW() WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL`,
		id, teamID,
	)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) CountOpenByAssignee(ctx context.Context, teamID, userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM conversations
		WHERE team_id = $1 AND assigned_to = $2 AND deleted_at IS NULL
			AND status IN ('open', 'pending', 'assigned')
	`, teamID, userID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *PostgresRepository) CountByTeamFilter(ctx context.Context, teamID uuid.UUID, filter ListFilter) (int64, error) {
	filter.View = filter.View
	page, err := r.List(ctx, teamID, filter, repository.PageParams{Page: 1, PageSize: 1})
	if err != nil {
		return 0, err
	}
	return page.Pagination.Total, nil
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, teamID, id uuid.UUID) error {
	tag, err := r.db.Pool.Exec(ctx,
		`UPDATE conversations SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND team_id = $2 AND deleted_at IS NULL`,
		id, teamID,
	)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

const conversationSelectCols = `
	id, team_id, contact_id, channel_id, channel_type, status, assigned_to,
	unread_count, last_message_preview, last_message_at,
	first_response_at, last_customer_message_at, last_agent_message_at, resolution_at,
	created_at, updated_at, deleted_at
`

func scanConversation(row pgx.Row) (*Conversation, error) { return scanConversationRow(row) }

func scanConversationRow(row pgx.Row) (*Conversation, error) {
	var c Conversation
	var channelType, status string
	err := row.Scan(
		&c.ID, &c.TeamID, &c.ContactID, &c.ChannelID, &channelType, &status, &c.AssignedTo,
		&c.UnreadCount, &c.LastMessagePreview, &c.LastMessageAt,
		&c.FirstResponseAt, &c.LastCustomerMessageAt, &c.LastAgentMessageAt, &c.ResolutionAt,
		&c.CreatedAt, &c.UpdatedAt, &c.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan conversation: %w", err)
	}
	c.ChannelType = ChannelType(channelType)
	c.Status = Status(status)
	return &c, nil
}
