package campaign

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type noopQuerier struct{}

func (noopQuerier) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (noopQuerier) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (noopQuerier) QueryRow(context.Context, string, ...any) pgx.Row {
	return nil
}

func TestPostgresRepository_NestedTransactionWithoutPool(t *testing.T) {
	repo := NewRepositoryWithQuerier(noopQuerier{})
	now := time.Now().UTC()
	c := &Campaign{
		ID:        uuid.New(),
		TeamID:    uuid.New(),
		Status:    StatusDraft,
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := repo.Create(context.Background(), c, nil)
	if !errors.Is(err, repository.ErrNestedTransaction) {
		t.Fatalf("Create() error = %v, want %v", err, repository.ErrNestedTransaction)
	}
}

func TestNewRepositoryWithQuerier_DetectsTransaction(t *testing.T) {
	var _ database.Querier = noopQuerier{}
	repo := NewRepositoryWithQuerier(noopQuerier{})
	if repo.inTx {
		t.Fatal("pool querier should not mark repository as inTx")
	}
}
