package message

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type AttachmentRepository interface {
	Create(ctx context.Context, attachment *MessageAttachment) error
	FindByMessageIDs(ctx context.Context, teamID uuid.UUID, messageIDs []uuid.UUID) (map[uuid.UUID]*Attachment, error)
}

type MessageAttachment struct {
	ID              uuid.UUID
	MessageID       uuid.UUID
	TeamID          uuid.UUID
	StorageObjectID uuid.UUID
	MimeType        string
	SizeBytes       int64
	Filename        string
}

type PostgresAttachmentRepository struct {
	db *database.DB
}

func NewAttachmentRepository(db *database.DB) *PostgresAttachmentRepository {
	return &PostgresAttachmentRepository{db: db}
}

func (r *PostgresAttachmentRepository) Create(ctx context.Context, attachment *MessageAttachment) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO message_attachments (
			id, message_id, team_id, storage_object_id, mime_type, size_bytes, filename, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,NOW())
	`, attachment.ID, attachment.MessageID, attachment.TeamID, attachment.StorageObjectID,
		attachment.MimeType, attachment.SizeBytes, attachment.Filename)
	return repository.MapError(err)
}

func (r *PostgresAttachmentRepository) FindByMessageIDs(ctx context.Context, teamID uuid.UUID, messageIDs []uuid.UUID) (map[uuid.UUID]*Attachment, error) {
	result := make(map[uuid.UUID]*Attachment)
	if len(messageIDs) == 0 {
		return result, nil
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT message_id, storage_object_id, mime_type, size_bytes, filename
		FROM message_attachments
		WHERE team_id = $1 AND message_id = ANY($2)
	`, teamID, messageIDs)
	if err != nil {
		return nil, fmt.Errorf("list message attachments: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var messageID, storageObjectID uuid.UUID
		var mimeType, filename string
		var sizeBytes int64
		if err := rows.Scan(&messageID, &storageObjectID, &mimeType, &sizeBytes, &filename); err != nil {
			return nil, err
		}
		result[messageID] = &Attachment{
			StorageObjectID: storageObjectID,
			MimeType:        mimeType,
			SizeBytes:       sizeBytes,
			Filename:        filename,
		}
	}
	return result, rows.Err()
}
