package product

import (
	"context"
	"net/http"
	"uuid"

	"chi-product-api/internal/database"
	db "chi-product-api/internal/database/sqlc"
	"chi-product-api/internal/platform/apperror"
	"chi-product-api/internal/tracing"

	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel/trace"
)

type Service struct {
	queries *db.Queries
	tracer  trace.Tracer
}

func NewService(
	queries *db.Queries,
	tracer trace.Tracer,
) *Service {
	return &Service{
		queries: queries,
		tracer:  tracer,
	}
}

func (s *Service) List(ctx context.Context, limit int, after *pageCursor) ([]db.Product, *pageCursor, error) {
	ctx, span := s.tracer.Start(ctx, "ProductService.List")
	defer span.End()

	params := db.ListProductsParams{RowLimit: int32(limit + 1)}
	if after != nil {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: after.CreatedAt, Valid: true}
		params.CursorID = pgtype.UUID{Bytes: after.ID, Valid: true}
	}

	products, err := s.queries.ListProducts(ctx, params)
	if err != nil {
		return nil, nil, tracing.Fail(span, apperror.Internal("PRODUCT_QUERY_FAILED", "failed to list products", err))
	}

	if len(products) <= limit {
		return products, nil, nil
	}

	products = products[:limit]
	last := products[limit-1]

	return products, &pageCursor{CreatedAt: last.CreatedAt.Time, ID: last.ID.Bytes}, nil
}

func (s *Service) FindByID(ctx context.Context, id uuid.UUID) (db.Product, error) {
	ctx, span := s.tracer.Start(ctx, "ProductService.FindByID")
	defer span.End()

	product, err := s.queries.GetProduct(ctx, pgUUID(id))
	if err != nil {
		return db.Product{}, mapError(span, err, "PRODUCT_QUERY_FAILED", "failed to get product")
	}

	return product, nil
}

func (s *Service) Create(ctx context.Context, req ProductRequest) (db.Product, error) {
	ctx, span := s.tracer.Start(ctx, "ProductService.Create")
	defer span.End()

	price, err := toNumeric(req.Price)
	if err != nil {
		return db.Product{}, err
	}

	product, err := s.queries.CreateProduct(ctx, db.CreateProductParams{
		Name:  req.Name,
		Price: price,
	})
	if err != nil {
		return db.Product{}, tracing.Fail(span, apperror.Internal("PRODUCT_CREATE_FAILED", "failed to create product", err))
	}

	return product, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req ProductRequest) (db.Product, error) {
	ctx, span := s.tracer.Start(ctx, "ProductService.Update")
	defer span.End()

	price, err := toNumeric(req.Price)
	if err != nil {
		return db.Product{}, err
	}

	product, err := s.queries.UpdateProduct(ctx, db.UpdateProductParams{
		ID:    pgUUID(id),
		Name:  req.Name,
		Price: price,
	})
	if err != nil {
		return db.Product{}, mapError(span, err, "PRODUCT_UPDATE_FAILED", "failed to update product")
	}

	return product, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "ProductService.Delete")
	defer span.End()

	if _, err := s.queries.DeleteProduct(ctx, pgUUID(id)); err != nil {
		return mapError(span, err, "PRODUCT_DELETE_FAILED", "failed to delete product")
	}

	return nil
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func toNumeric(value string) (pgtype.Numeric, error) {
	var price pgtype.Numeric

	if err := price.Scan(value); err != nil {
		return price, apperror.Wrap(
			http.StatusBadRequest,
			"INVALID_PRODUCT_PRICE",
			"invalid product price",
			err,
		)
	}

	return price, nil
}

// mapError turns a query error into the API error for a single-product operation.
func mapError(span trace.Span, err error, code, message string) error {
	if database.IsNotFound(err) {
		return apperror.NotFound("PRODUCT_NOT_FOUND", "product not found")
	}

	return tracing.Fail(span, apperror.Internal(code, message, err))
}
