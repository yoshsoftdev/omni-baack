package conversation

import (
	"context"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

func (s *Service) loadContactsByConversations(ctx context.Context, teamID uuid.UUID, convs []Conversation) (map[uuid.UUID]contact.Contact, error) {
	if len(convs) == 0 {
		return map[uuid.UUID]contact.Contact{}, nil
	}
	ids := make([]uuid.UUID, 0, len(convs))
	seen := make(map[uuid.UUID]struct{}, len(convs))
	for _, conv := range convs {
		if _, ok := seen[conv.ContactID]; ok {
			continue
		}
		seen[conv.ContactID] = struct{}{}
		ids = append(ids, conv.ContactID)
	}
	contacts, err := s.contacts.ListByIDs(ctx, teamID, ids)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	out := make(map[uuid.UUID]contact.Contact, len(contacts))
	for _, ct := range contacts {
		out[ct.ID] = ct
	}
	return out, nil
}

func (s *Service) enrichOne(ctx context.Context, teamID uuid.UUID, conv *Conversation) (*Item, error) {
	if conv == nil {
		return nil, nil
	}
	contactMap, err := s.loadContactsByConversations(ctx, teamID, []Conversation{*conv})
	if err != nil {
		return nil, err
	}
	item := toItem(*conv, contactMap)
	return &item, nil
}

func (s *Service) enrichPage(ctx context.Context, teamID uuid.UUID, page repository.Page[Conversation]) (repository.Page[Item], error) {
	contactMap, err := s.loadContactsByConversations(ctx, teamID, page.Items)
	if err != nil {
		return repository.Page[Item]{}, err
	}
	return repository.Page[Item]{
		Items:      toItems(page.Items, contactMap),
		Pagination: page.Pagination,
	}, nil
}
