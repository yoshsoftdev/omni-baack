package campaign

import "context"

// UnitOfWorkRunner executes campaign repository work inside a single database transaction.
type UnitOfWorkRunner interface {
	Run(ctx context.Context, fn func(ctx context.Context, repos Repository) error) error
}
