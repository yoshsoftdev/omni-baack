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
	if err := s.attachTemplateEnrichment(ctx, teamID, pageResult.Items, enrichment); err != nil {
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
	if err := s.attachTemplateEnrichment(ctx, teamID, []Campaign{*c}, enrichment); err != nil {
		return nil, err
	}
	resp := NewCampaignResponse(*c, enrichment[c.ID])
	return &resp, nil
}

// attachTemplateEnrichment fills template name/language via TemplateRepository only.
func (s *Service) attachTemplateEnrichment(
	ctx context.Context,
	teamID uuid.UUID,
	campaigns []Campaign,
	enrichment map[uuid.UUID]ListEnrichment,
) error {
	if s.templates == nil || len(campaigns) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(campaigns))
	seen := map[uuid.UUID]struct{}{}
	for _, c := range campaigns {
		if _, ok := seen[c.TemplateID]; ok {
			continue
		}
		seen[c.TemplateID] = struct{}{}
		ids = append(ids, c.TemplateID)
	}
	templates, err := s.templates.FindByIDs(ctx, teamID, ids)
	if err != nil {
		return err
	}
	for _, c := range campaigns {
		tmpl, ok := templates[c.TemplateID]
		if !ok || tmpl == nil {
			continue
		}
		item := enrichment[c.ID]
		item.TemplateName = tmpl.Name
		item.TemplateLanguage = tmpl.Language
		enrichment[c.ID] = item
	}
	return nil
}
