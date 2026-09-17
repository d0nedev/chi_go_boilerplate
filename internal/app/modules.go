package app

import (
	"chi-product-api/internal/config"
	db "chi-product-api/internal/database/sqlc"
	"chi-product-api/internal/middleware"
	"chi-product-api/internal/product"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"
)

// modules wires every domain and returns the routes each one mounts under /api/v1.
// To add a domain: build its service/handler here and append its RegisterRoutes.
func modules(
	cfg *config.Config,
	logger *slog.Logger,
	pool *pgxpool.Pool,
	tp trace.TracerProvider,
) []func(chi.Router) {
	queries := db.New(pool)
	requireAPIKey := middleware.APIKey(cfg.Auth.APIKeys)

	products := product.NewHandler(
		product.NewService(queries, tp.Tracer("chi-product-api/internal/product")),
	)

	return []func(chi.Router){
		func(r chi.Router) { product.RegisterRoutes(r, products, logger, requireAPIKey) },
	}
}
