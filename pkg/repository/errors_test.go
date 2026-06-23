package repository

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapErrorNotFound(t *testing.T) {
	if !errors.Is(MapError(pgx.ErrNoRows), ErrNotFound) {
		t.Fatal("expected ErrNotFound for pgx.ErrNoRows")
	}
}

func TestMapErrorConflict(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23505", ConstraintName: "uq_teams_slug_active"}
	if !errors.Is(MapError(pgErr), ErrConflict) {
		t.Fatal("expected ErrConflict for unique violation")
	}
}

func TestIsNotFound(t *testing.T) {
	if !IsNotFound(ErrNotFound) || !IsNotFound(pgx.ErrNoRows) {
		t.Fatal("IsNotFound should match repository and pgx not-found errors")
	}
}
