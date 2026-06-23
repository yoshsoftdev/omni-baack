package uow

import (
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

// RepositoryFactory builds transaction-scoped repository implementations.
type RepositoryFactory interface {
	Campaigns(q database.Querier) campaign.Repository
}

type factory struct{}

// NewFactory returns a RepositoryFactory for UnitOfWork.
func NewFactory() RepositoryFactory {
	return &factory{}
}

func (f *factory) Campaigns(q database.Querier) campaign.Repository {
	return campaign.NewRepositoryWithQuerier(q)
}
