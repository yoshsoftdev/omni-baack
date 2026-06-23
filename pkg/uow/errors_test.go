package uow

import (
	"errors"
	"testing"

	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

func TestErrNestedTransactionReexport(t *testing.T) {
	if !errors.Is(ErrNestedTransaction, repository.ErrNestedTransaction) {
		t.Fatal("uow.ErrNestedTransaction should match repository.ErrNestedTransaction")
	}
}
