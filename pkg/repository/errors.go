package repository

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("conflict")
	ErrInvalidInput      = errors.New("invalid input")
	ErrLimitExceeded     = errors.New("limit exceeded")
	ErrNestedTransaction = errors.New("nested transaction not allowed")
)

// IsNotFound reports whether err is a not-found repository error.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, pgx.ErrNoRows)
}

// MapError normalizes pgx and constraint errors into repository errors.
func MapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: %s", ErrConflict, pgErr.ConstraintName)
		case "23503":
			return fmt.Errorf("%w: %s", ErrInvalidInput, pgErr.ConstraintName)
		case "23514":
			return fmt.Errorf("%w: %s", ErrInvalidInput, pgErr.ConstraintName)
		case "22001":
			return fmt.Errorf("%w: value too long for %s", ErrInvalidInput, pgErr.ColumnName)
		}
	}
	if strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
		return ErrConflict
	}
	return err
}
