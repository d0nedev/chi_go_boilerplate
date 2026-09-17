package database

import (
	"errors"

	"github.com/jackc/pgx/v5"
)

// IsNotFound reports whether a :one query matched no rows.
func IsNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
