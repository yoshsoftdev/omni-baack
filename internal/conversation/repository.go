package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
	ListCursor(ctx context.Context, teamID uuid.UUID, filter ListFilter, cursor *ListCursor, limit int) (CursorPage, error)
	UpdateStatus(ctx context.Context, teamID, id uuid.UUID, status Status) error
	UpdateAssignment(ctx context.Context, teamID, id uuid.UUID, assignedTo *uuid.UUID) error
	UpdateUnreadCount(ctx context.Context, teamID, id uuid.UUID, count int) error
	UpdateLastMessage(ctx context.Context, teamID, id uuid.UUID, update LastMessageUpdate) error
	ApplyInboxSync(ctx context.Context, teamID, id uuid.UUID, update InboxSyncUpdate) error
	UpdatePriorityFields(ctx context.Context, teamID, id uuid.UUID, waitingSince *time.Time, clearWaiting bool, basePriority int, priorityScore int64) error
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
			id, team_id, contact_id, provider_profile_id, external_thread_id, channel_id, channel_type, status, assigned_to,
			unread_count, last_message_preview, last_message_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`, c.ID, c.TeamID, c.ContactID, c.ProviderProfileID, c.ExternalThreadID, c.ChannelID, string(c.ChannelType), string(c.Status),
		c.AssignedTo, c.UnreadCount, c.LastMessagePreview, c.LastMessageAt, c.CreatedAt, c.UpdatedAt)
	return repository.MapError(err)
}

func (r *PostgresRepository) FindByID(ctx context.Context, teamID, id uuid.UUID) (*Conversation, error) {
	return scanConversation(r.db.Pool.QueryRow(ctx, `
		SELECT `+conversationSelectCols+`
		FROM `+conversationFromClause+`
		WHERE c.id = $1 AND c.team_id = $2 AND c.deleted_at IS NULL
	`, id, teamID))
}

func (r *PostgresRepository) FindByContactChannel(ctx context.Context, teamID, contactID uuid.UUID, channelID *uuid.UUID) (*Conversation, error) {
	return scanConversation(r.db.Pool.QueryRow(ctx, `
		SELECT `+conversationSelectCols+`
		FROM `+conversationFromClause+`
		WHERE c.team_id = $1
		  AND c.channel_id IS NOT DISTINCT FROM $3
		  AND c.contact_id = $2
		  AND c.deleted_at IS NULL
		ORDER BY c.last_message_at DESC NULLS LAST, c.created_at DESC
		LIMIT 1
	`, teamID, contactID, channelID))
}

func (r *PostgresRepository) List(ctx context.Context, teamID uuid.UUID, filter ListFilter, page repository.PageParams) (repository.Page[Conversation], error) {
	page = page.Normalize()
	args := []any{teamID}
	where := `c.team_id = $1 AND c.deleted_at IS NULL`
	if filter.Status != nil {
		args = append(args, string(*filter.Status))
		where += fmt.Sprintf(` AND c.status = $%d`, len(args))
	}
	if filter.AssignedTo != nil {
		args = append(args, *filter.AssignedTo)
		where += fmt.Sprintf(` AND c.assigned_to = $%d`, len(args))
	}
	if filter.ChannelID != nil {
		args = append(args, *filter.ChannelID)
		where += fmt.Sprintf(` AND c.channel_id = $%d`, len(args))
	}
	if filter.ChannelType != nil {
		args = append(args, string(*filter.ChannelType))
		where += fmt.Sprintf(` AND c.channel_type = $%d`, len(args))
	}
	switch filter.View {
	case ViewMine:
		if filter.ActorID != nil {
			args = append(args, *filter.ActorID)
			where += fmt.Sprintf(` AND c.assigned_to = $%d`, len(args))
		}
	case ViewUnassigned:
		where += ` AND c.assigned_to IS NULL`
	case ViewUnread:
		where += ` AND c.unread_count > 0`
	case ViewClosed:
		where += ` AND c.status IN ('closed', 'resolved')`
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		where += fmt.Sprintf(` AND c.last_message_preview ILIKE $%d`, len(args))
	}
	var total int64
	if err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM `+conversationFromClause+` WHERE `+where, args...).Scan(&total); err != nil {
		return repository.Page[Conversation]{}, err
	}
	args = append(args, page.Limit(), page.Offset())
	rows, err := r.db.Pool.Query(ctx, fmt.Sprintf(`
		SELECT `+conversationSelectCols+`
		FROM `+conversationFromClause+` WHERE %s ORDER BY c.last_message_at DESC NULLS LAST, c.created_at DESC
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

func (r *PostgresRepository) ListCursor(ctx context.Context, teamID uuid.UUID, filter ListFilter, cursor *ListCursor, limit int) (CursorPage, error) {
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	fetchLimit := limit + 1

	fromClause, where, args := buildInboxListQuery(teamID, filter)
	if cursor != nil {
		cursorIdx := len(args) + 1
		args = append(args, cursor.PriorityScore, cursor.LastMessageAt, cursor.ID)
		where += fmt.Sprintf(`
			AND (
				c.priority_score < $%d
				OR (c.priority_score = $%d AND c.last_message_at < $%d)
				OR (c.priority_score = $%d AND c.last_message_at IS NOT DISTINCT FROM $%d AND c.id < $%d)
				OR (c.priority_score = $%d AND c.last_message_at IS NULL AND $%d IS NOT NULL)
			)`,
			cursorIdx, cursorIdx, cursorIdx+1,
			cursorIdx, cursorIdx+1, cursorIdx+2,
			cursorIdx, cursorIdx+1,
		)
	}
	args = append(args, fetchLimit)
	limitParam := len(args)

	query := fmt.Sprintf(`
		SELECT %s
		FROM %s
		WHERE %s
		ORDER BY c.priority_score DESC, c.last_message_at DESC NULLS LAST, c.id DESC
		LIMIT $%d
	`, conversationSelectColsAliased, fromClause, where, limitParam)

	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return CursorPage{}, err
	}
	defer rows.Close()

	items := make([]Conversation, 0, fetchLimit)
	for rows.Next() {
		c, err := scanConversationRow(rows)
		if err != nil {
			return CursorPage{}, err
		}
		items = append(items, *c)
	}
	if err := rows.Err(); err != nil {
		return CursorPage{}, err
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	var next *ListCursor
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		next = &ListCursor{
			PriorityScore: last.PriorityScore,
			LastMessageAt: last.LastMessageAt,
			ID:            last.ID,
		}
	}

	return CursorPage{Items: items, NextCursor: next, HasMore: hasMore}, nil
}

func buildInboxListQuery(teamID uuid.UUID, filter ListFilter) (fromClause, where string, args []any) {
	args = []any{teamID}
	fromClause = conversationFromClause
	where = `c.team_id = $1 AND c.deleted_at IS NULL`

	if filter.Status != nil {
		args = append(args, string(*filter.Status))
		where += fmt.Sprintf(` AND c.status = $%d`, len(args))
	}
	if filter.AssignedTo != nil {
		args = append(args, *filter.AssignedTo)
		where += fmt.Sprintf(` AND c.assigned_to = $%d`, len(args))
	}
	if filter.ChannelID != nil {
		args = append(args, *filter.ChannelID)
		where += fmt.Sprintf(` AND c.channel_id = $%d`, len(args))
	}
	if filter.ChannelType != nil {
		args = append(args, string(*filter.ChannelType))
		where += fmt.Sprintf(` AND c.channel_type = $%d`, len(args))
	}
	switch filter.View {
	case ViewMine:
		if filter.ActorID != nil {
			args = append(args, *filter.ActorID)
			where += fmt.Sprintf(` AND c.assigned_to = $%d`, len(args))
		}
	case ViewUnassigned:
		where += ` AND c.assigned_to IS NULL`
	case ViewUnread:
		where += ` AND c.unread_count > 0`
	case ViewClosed:
		where += ` AND c.status IN ('closed', 'resolved')`
	}
	if strings.TrimSpace(filter.Search) != "" {
		pattern := "%" + strings.TrimSpace(filter.Search) + "%"
		args = append(args, pattern)
		idx := len(args)
		where += fmt.Sprintf(` AND (
			c.last_message_preview ILIKE $%d
			OR owner.contact_name ILIKE $%d
			OR owner.customer_name ILIKE $%d
			OR owner.phone ILIKE $%d
			OR EXISTS (
				SELECT 1 FROM provider_profiles search_profile
				WHERE search_profile.team_id = c.team_id
				  AND search_profile.contact_id = COALESCE(pp.contact_id, c.contact_id)
				  AND search_profile.deleted_at IS NULL
				  AND (search_profile.display_name ILIKE $%d OR search_profile.username ILIKE $%d OR search_profile.phone ILIKE $%d)
			)
		)`, idx, idx, idx, idx, idx, idx, idx)
	}
	return fromClause, where, args
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
			last_message_at = CASE WHEN $10 THEN $4 ELSE last_message_at END,
			last_customer_message_at = COALESCE($5, last_customer_message_at),
			last_agent_message_at = COALESCE($6, last_agent_message_at),
			first_response_at = CASE WHEN $7 THEN COALESCE(first_response_at, NOW()) ELSE first_response_at END,
			resolution_at = CASE
				WHEN $8 THEN NULL
				WHEN $9 THEN NOW()
				ELSE resolution_at
			END,
			waiting_since = CASE
				WHEN $11 THEN NULL
				WHEN $12::timestamptz IS NOT NULL THEN $12::timestamptz
				ELSE waiting_since
			END,
			base_priority = COALESCE($13, base_priority),
			priority_score = COALESCE($14, priority_score),
			updated_at = NOW()
		WHERE id = $15 AND team_id = $16 AND deleted_at IS NULL
	`
	tag, err := r.db.Pool.Exec(ctx, query,
		status, unread, u.LastMessagePreview, u.LastMessageAt,
		u.LastCustomerMessageAt, u.LastAgentMessageAt,
		u.SetFirstResponseAt, u.ClearResolutionAt, u.SetResolutionAt,
		u.TouchLastMessage,
		u.ClearWaitingSince, u.WaitingSince, u.BasePriority, u.PriorityScore,
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

func (r *PostgresRepository) UpdatePriorityFields(ctx context.Context, teamID, id uuid.UUID, waitingSince *time.Time, clearWaiting bool, basePriority int, priorityScore int64) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE conversations SET
			waiting_since = CASE WHEN $1 THEN NULL ELSE $2::timestamptz END,
			base_priority = $3,
			priority_score = $4,
			updated_at = NOW()
		WHERE id = $5 AND team_id = $6 AND deleted_at IS NULL
	`, clearWaiting, waitingSince, basePriority, priorityScore, id, teamID)
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
	c.id, c.team_id, COALESCE(pp.contact_id, c.contact_id), c.provider_profile_id, c.external_thread_id,
	COALESCE(pp.provider, c.channel_type::text),
	ARRAY(
		SELECT DISTINCT linked.provider
		FROM provider_profiles linked
		WHERE linked.team_id = c.team_id
		  AND linked.contact_id = COALESCE(pp.contact_id, c.contact_id)
		  AND linked.deleted_at IS NULL
		ORDER BY linked.provider
	),
	COALESCE(owner.source_of_truth_profile_id = c.provider_profile_id, false),
	(
		SELECT COUNT(*)::int
		FROM provider_profiles linked
		WHERE linked.team_id = c.team_id
		  AND linked.contact_id = COALESCE(pp.contact_id, c.contact_id)
		  AND linked.deleted_at IS NULL
	),
	c.channel_id, c.channel_type, c.status, c.assigned_to,
	c.unread_count, c.last_message_preview, c.last_message_at,
	c.first_response_at, c.last_customer_message_at, c.last_agent_message_at, c.resolution_at,
	c.waiting_since, c.base_priority, c.priority_score,
	c.created_at, c.updated_at, c.deleted_at
`

const conversationSelectColsAliased = conversationSelectCols

const conversationFromClause = `
	conversations c
	LEFT JOIN provider_profiles pp
	  ON pp.id = c.provider_profile_id AND pp.team_id = c.team_id AND pp.deleted_at IS NULL
	LEFT JOIN contacts owner
	  ON owner.id = COALESCE(pp.contact_id, c.contact_id)
	 AND owner.team_id = c.team_id
	 AND owner.deleted_at IS NULL
`

func scanConversation(row pgx.Row) (*Conversation, error) { return scanConversationRow(row) }

func scanConversationRow(row pgx.Row) (*Conversation, error) {
	var c Conversation
	var channelType, status string
	err := row.Scan(
		&c.ID, &c.TeamID, &c.ContactID, &c.ProviderProfileID, &c.ExternalThreadID, &c.Provider,
		&c.LinkedProviders, &c.IsSourceOfTruth, &c.LinkedAccountCount,
		&c.ChannelID, &channelType, &status, &c.AssignedTo,
		&c.UnreadCount, &c.LastMessagePreview, &c.LastMessageAt,
		&c.FirstResponseAt, &c.LastCustomerMessageAt, &c.LastAgentMessageAt, &c.ResolutionAt,
		&c.WaitingSince, &c.BasePriority, &c.PriorityScore,
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
