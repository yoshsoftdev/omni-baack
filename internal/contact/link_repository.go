package contact

import (
	"bytes"
	"context"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// LinkRepository persists symmetric relationships between independent contacts.
type LinkRepository interface {
	ListByContactIDs(ctx context.Context, teamID uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID][]LinkedContact, error)
	Link(ctx context.Context, teamID, contactID, linkedContactID, actorID uuid.UUID) error
	Unlink(ctx context.Context, teamID, contactID, linkedContactID uuid.UUID) error
}

type PostgresLinkRepository struct {
	db *database.DB
}

func NewLinkRepository(db *database.DB) *PostgresLinkRepository {
	return &PostgresLinkRepository{db: db}
}

func (r *PostgresLinkRepository) ListByContactIDs(
	ctx context.Context,
	teamID uuid.UUID,
	contactIDs []uuid.UUID,
) (map[uuid.UUID][]LinkedContact, error) {
	result := make(map[uuid.UUID][]LinkedContact)
	if len(contactIDs) == 0 {
		return result, nil
	}

	rows, err := r.db.Pool.Query(ctx, `
		SELECT owner_id, link_id, linked_id, contact_name, customer_name, phone, email,
		       avatar_url, provider_kinds, linked_at
		FROM (
			SELECT
				cl.contact_a_id AS owner_id,
				cl.id AS link_id,
				b.id AS linked_id,
				b.contact_name,
				b.customer_name,
				COALESCE(b.phone, '') AS phone,
				b.email,
				b.avatar_url,
				ARRAY(
					SELECT DISTINCT p.provider
					FROM provider_profiles p
					WHERE p.team_id = cl.team_id
					  AND p.contact_id = b.id
					  AND p.deleted_at IS NULL
					ORDER BY p.provider
				) AS provider_kinds,
				cl.created_at AS linked_at
			FROM contact_links cl
			JOIN contacts a
			  ON a.id = cl.contact_a_id AND a.team_id = cl.team_id AND a.deleted_at IS NULL
			JOIN contacts b
			  ON b.id = cl.contact_b_id AND b.team_id = cl.team_id AND b.deleted_at IS NULL
			WHERE cl.team_id = $1
			  AND cl.contact_a_id = ANY($2::uuid[])
			  AND cl.deleted_at IS NULL

			UNION ALL

			SELECT
				cl.contact_b_id AS owner_id,
				cl.id AS link_id,
				a.id AS linked_id,
				a.contact_name,
				a.customer_name,
				COALESCE(a.phone, '') AS phone,
				a.email,
				a.avatar_url,
				ARRAY(
					SELECT DISTINCT p.provider
					FROM provider_profiles p
					WHERE p.team_id = cl.team_id
					  AND p.contact_id = a.id
					  AND p.deleted_at IS NULL
					ORDER BY p.provider
				) AS provider_kinds,
				cl.created_at AS linked_at
			FROM contact_links cl
			JOIN contacts a
			  ON a.id = cl.contact_a_id AND a.team_id = cl.team_id AND a.deleted_at IS NULL
			JOIN contacts b
			  ON b.id = cl.contact_b_id AND b.team_id = cl.team_id AND b.deleted_at IS NULL
			WHERE cl.team_id = $1
			  AND cl.contact_b_id = ANY($2::uuid[])
			  AND cl.deleted_at IS NULL
		) linked
		ORDER BY linked_at ASC, linked_id ASC
	`, teamID, contactIDs)
	if err != nil {
		return nil, repository.MapError(err)
	}
	defer rows.Close()

	for rows.Next() {
		var ownerID uuid.UUID
		var item LinkedContact
		if err := rows.Scan(
			&ownerID,
			&item.LinkID,
			&item.ID,
			&item.ContactName,
			&item.CustomerName,
			&item.Phone,
			&item.Email,
			&item.AvatarURL,
			&item.ProviderKinds,
			&item.LinkedAt,
		); err != nil {
			return nil, repository.MapError(err)
		}
		result[ownerID] = append(result[ownerID], item)
	}
	if err := rows.Err(); err != nil {
		return nil, repository.MapError(err)
	}
	return result, nil
}

func (r *PostgresLinkRepository) Link(
	ctx context.Context,
	teamID, contactID, linkedContactID, actorID uuid.UUID,
) error {
	contactA, contactB := orderedContactIDs(contactID, linkedContactID)
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO contact_links (
			id, team_id, contact_a_id, contact_b_id, created_by, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,NOW(),NOW())
		ON CONFLICT (team_id, contact_a_id, contact_b_id)
			WHERE deleted_at IS NULL
		DO NOTHING
	`, uuid.New(), teamID, contactA, contactB, actorID)
	return repository.MapError(err)
}

func (r *PostgresLinkRepository) Unlink(
	ctx context.Context,
	teamID, contactID, linkedContactID uuid.UUID,
) error {
	contactA, contactB := orderedContactIDs(contactID, linkedContactID)
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE contact_links
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE team_id = $1
		  AND contact_a_id = $2
		  AND contact_b_id = $3
		  AND deleted_at IS NULL
	`, teamID, contactA, contactB)
	if err != nil {
		return repository.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func orderedContactIDs(left, right uuid.UUID) (uuid.UUID, uuid.UUID) {
	if bytes.Compare(left[:], right[:]) < 0 {
		return left, right
	}
	return right, left
}

var _ LinkRepository = (*PostgresLinkRepository)(nil)
