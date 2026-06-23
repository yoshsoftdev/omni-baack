package uow

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

// TransactionManager coordinates a single database transaction across repositories.
type TransactionManager interface {
	Run(ctx context.Context, fn func(ctx context.Context, uow UnitOfWork) error) error
}

type manager struct {
	db      *database.DB
	factory RepositoryFactory
}

// NewManager returns a TransactionManager backed by the database pool.
func NewManager(db *database.DB, factory RepositoryFactory) TransactionManager {
	return &manager{db: db, factory: factory}
}

func (m *manager) Run(ctx context.Context, fn func(ctx context.Context, uow UnitOfWork) error) error {
	tx, err := m.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	var runErr error
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
		if runErr != nil {
			_ = tx.Rollback(ctx)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			runErr = fmt.Errorf("commit tx: %w", err)
		}
	}()

	uow := newUnitOfWork(tx, m.factory)
	runErr = fn(ctx, uow)
	return runErr
}

// RunInExistingTx runs fn on an existing transaction (for tests).
func RunInExistingTx(ctx context.Context, tx pgx.Tx, factory RepositoryFactory, fn func(ctx context.Context, uow UnitOfWork) error) error {
	return fn(ctx, newUnitOfWork(tx, factory))
}
