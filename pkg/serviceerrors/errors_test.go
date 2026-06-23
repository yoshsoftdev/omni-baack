package serviceerrors

import (
	"errors"
	"testing"

	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

func TestMapRepository(t *testing.T) {
	if !errors.Is(MapRepository(repository.ErrNotFound), ErrNotFound) {
		t.Fatal("expected ErrNotFound")
	}
	if !errors.Is(MapRepository(repository.ErrConflict), ErrConflict) {
		t.Fatal("expected ErrConflict")
	}
}

func TestValidationErrorIsValidation(t *testing.T) {
	err := NewValidation("phone", "invalid")
	if !IsValidation(err) {
		t.Fatal("expected validation error")
	}
}
