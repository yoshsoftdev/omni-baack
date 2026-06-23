package uow

import "github.com/omnichannel/omni-chan-v2/pkg/repository"

// ErrNestedTransaction is returned when a repository tries to begin a transaction
// while already running on a UnitOfWork transaction.
var ErrNestedTransaction = repository.ErrNestedTransaction
