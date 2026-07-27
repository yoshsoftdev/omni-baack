package campaign

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (r *PostgresRepository) LoadListEnrichment(
	ctx context.Context,
	teamID uuid.UUID,
	campaigns []Campaign,
) (map[uuid.UUID]ListEnrichment, error) {
	out := make(map[uuid.UUID]ListEnrichment, len(campaigns))
	if len(campaigns) == 0 {
		return out, nil
	}

	campaignIDs := make([]uuid.UUID, 0, len(campaigns))
	for _, c := range campaigns {
		campaignIDs = append(campaignIDs, c.ID)
	}

	// Template name/language are filled by Campaign Service via template.Repository
	// (no direct SQL against templates outside the template package).
	sources, err := r.loadAudienceSources(ctx, campaignIDs)
	if err != nil {
		return nil, err
	}
	snapshots, err := r.loadLatestSnapshots(ctx, campaignIDs)
	if err != nil {
		return nil, err
	}

	for _, c := range campaigns {
		item := ListEnrichment{
			Sources: sources[c.ID],
		}
		if snap, ok := snapshots[c.ID]; ok {
			item.SnapshotAt = snap.SnapshotAt
			item.SnapshotResolved = snap.ResolvedCount
			item.HasSnapshot = true
		}
		out[c.ID] = item
	}

	return out, nil
}

func (r *PostgresRepository) loadAudienceSources(ctx context.Context, campaignIDs []uuid.UUID) (map[uuid.UUID][]AudienceSourceView, error) {
	out := make(map[uuid.UUID][]AudienceSourceView, len(campaignIDs))
	if len(campaignIDs) == 0 {
		return out, nil
	}
	rows, err := r.q.Query(ctx, `
		SELECT
			ca.campaign_id,
			ca.source_type,
			ca.source_id,
			COALESCE(s.name, tg.name, 'Manual contact') AS source_name
		FROM campaign_audiences ca
		LEFT JOIN segments s ON ca.source_type = 'segment' AND ca.source_id = s.id AND s.deleted_at IS NULL
		LEFT JOIN tags tg ON ca.source_type = 'tag' AND ca.source_id = tg.id AND tg.deleted_at IS NULL
		WHERE ca.campaign_id = ANY($1)
		ORDER BY ca.created_at ASC
	`, campaignIDs)
	if err != nil {
		if isMissingRelation(err) {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var campaignID, sourceID uuid.UUID
		var sourceType, sourceName string
		if err := rows.Scan(&campaignID, &sourceType, &sourceID, &sourceName); err != nil {
			return nil, err
		}
		if sourceType == string(AudienceContact) {
			sourceName = "Manual contact"
		}
		out[campaignID] = append(out[campaignID], AudienceSourceView{
			Type: sourceType,
			ID:   sourceID.String(),
			Name: sourceName,
		})
	}
	return out, rows.Err()
}

type snapshotMeta struct {
	SnapshotAt    *time.Time
	ResolvedCount int
}

func (r *PostgresRepository) loadLatestSnapshots(ctx context.Context, campaignIDs []uuid.UUID) (map[uuid.UUID]snapshotMeta, error) {
	out := make(map[uuid.UUID]snapshotMeta, len(campaignIDs))
	if len(campaignIDs) == 0 {
		return out, nil
	}
	rows, err := r.q.Query(ctx, `
		SELECT DISTINCT ON (campaign_id)
			campaign_id, snapshot_at, resolved_count
		FROM campaign_audience_snapshots
		WHERE campaign_id = ANY($1)
		ORDER BY campaign_id, snapshot_at DESC
	`, campaignIDs)
	if err != nil {
		if isMissingRelation(err) {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var campaignID uuid.UUID
		var meta snapshotMeta
		if err := rows.Scan(&campaignID, &meta.SnapshotAt, &meta.ResolvedCount); err != nil {
			return nil, err
		}
		out[campaignID] = meta
	}
	return out, rows.Err()
}

func isMissingRelation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") && strings.Contains(msg, "relation")
}
