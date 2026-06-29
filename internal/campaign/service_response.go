package campaign

import (
	"context"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

func (s *Service) ListResponses(
	ctx context.Context,
	teamID, userID uuid.UUID,
	actorRole string,
	filter ListFilter,
	page repository.PageParams,
) (repository.Page[CampaignResponse], error) {
	pageResult, err := s.List(ctx, teamID, userID, actorRole, filter, page)
	if err != nil {
		return repository.Page[CampaignResponse]{}, err
	}
	enrichment, err := s.campaigns.LoadListEnrichment(ctx, teamID, pageResult.Items)
	if err != nil {
		return repository.Page[CampaignResponse]{}, err
	}
	items := make([]CampaignResponse, 0, len(pageResult.Items))
	for _, c := range pageResult.Items {
		items = append(items, NewCampaignResponse(c, enrichment[c.ID]))
	}
	return repository.Page[CampaignResponse]{
		Items:      items,
		Pagination: pageResult.Pagination,
	}, nil
}

func (s *Service) GetResponse(ctx context.Context, teamID, userID uuid.UUID, actorRole string, campaignID uuid.UUID) (*CampaignResponse, error) {
	c, err := s.Get(ctx, teamID, userID, actorRole, campaignID)
	if err != nil {
		return nil, err
	}
	enrichment, err := s.campaigns.LoadListEnrichment(ctx, teamID, []Campaign{*c})
	if err != nil {
		return nil, err
	}
	resp := NewCampaignResponse(*c, enrichment[c.ID])
	return &resp, nil
}
