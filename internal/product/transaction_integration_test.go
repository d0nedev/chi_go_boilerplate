package product

import (
	"context"
	"errors"
	"testing"

	db "github.com/d0nedev/chi_go_boilerplate/internal/platform/database/sqlc"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Reference pattern for multi-statement writes: pgx.BeginFunc commits when fn
// returns nil and rolls back on error or panic; queries.WithTx scopes sqlc to the tx.
func TestIntegrationTransactionPattern(t *testing.T) {
	pool := newIntegrationPool(t)
	queries := db.New(pool)
	ctx := context.Background()

	var price pgtype.Numeric
	_ = price.Scan("1.00")

	count := func() int {
		rows, err := queries.ListProducts(ctx, db.ListProductsParams{RowLimit: 100})
		if err != nil {
			t.Fatal(err)
		}
		return len(rows)
	}

	errAbort := errors.New("abort")
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := queries.WithTx(tx)
		if _, err := q.CreateProduct(ctx, db.CreateProductParams{Name: "a", Price: price}); err != nil {
			return err
		}
		if _, err := q.CreateProduct(ctx, db.CreateProductParams{Name: "b", Price: price}); err != nil {
			return err
		}
		return errAbort
	})
	if !errors.Is(err, errAbort) || count() != 0 {
		t.Fatalf("rollback on error: err=%v rows=%d", err, count())
	}

	func() {
		defer func() { _ = recover() }()
		_ = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			_, _ = queries.WithTx(tx).CreateProduct(ctx, db.CreateProductParams{Name: "p", Price: price})
			panic("boom")
		})
	}()
	if count() != 0 {
		t.Fatalf("rollback on panic: rows=%d", count())
	}

	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		_, err := queries.WithTx(tx).CreateProduct(ctx, db.CreateProductParams{Name: "c", Price: price})
		return err
	})
	if err != nil || count() != 1 {
		t.Fatalf("commit: err=%v rows=%d", err, count())
	}
}
