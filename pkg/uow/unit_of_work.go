package uow

import (
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
)

// UnitOfWork provides repositories that share a single database transaction.
type UnitOfWork interface {
	Campaigns() campaign.Repository
	Tx() pgx.Tx
}

type unitOfWork struct {
	tx      pgx.Tx
	factory RepositoryFactory

	campaigns campaign.Repository
}

func newUnitOfWork(tx pgx.Tx, factory RepositoryFactory) UnitOfWork {
	return &unitOfWork{tx: tx, factory: factory}
}

func (u *unitOfWork) Campaigns() campaign.Repository {
	if u.campaigns == nil {
		u.campaigns = u.factory.Campaigns(u.tx)
	}
	return u.campaigns
}

func (u *unitOfWork) Tx() pgx.Tx {
	return u.tx
}
