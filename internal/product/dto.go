package product

import (
	"encoding/base64"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	db "chi-product-api/internal/database/sqlc"
	"chi-product-api/internal/platform/apperror"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	maxNameLength = 255
	defaultLimit  = 20
	maxLimit      = 100
)

// Matches NUMERIC(15,2): non-negative, up to 13 integer digits and 2 decimals.
var pricePattern = regexp.MustCompile(`^\d{1,13}(\.\d{1,2})?$`)

type ProductRequest struct {
	Name  string `json:"name"`
	Price string `json:"price"`
}

type ProductResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Price     string `json:"price"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type ListProductsResponse struct {
	Data       []ProductResponse `json:"data"`
	NextCursor *string           `json:"next_cursor"`
}

func (req ProductRequest) Validate() error {
	name := strings.TrimSpace(req.Name)

	switch {
	case name == "":
		return apperror.Validation("name is required")
	case utf8.RuneCountInString(name) > maxNameLength:
		return apperror.Validation("name must be at most 255 characters")
	case strings.TrimSpace(req.Price) == "":
		return apperror.Validation("price is required")
	case !pricePattern.MatchString(req.Price):
		return apperror.Validation("price must be a non-negative decimal string with at most 2 decimal places, e.g. \"12500.00\"")
	}

	return nil
}

func toProductResponse(product db.Product) ProductResponse {
	return ProductResponse{
		ID:        product.ID.String(),
		Name:      product.Name,
		Price:     numericString(product.Price),
		CreatedAt: product.CreatedAt.Time.UTC().Format(time.RFC3339),
		UpdatedAt: product.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}
}

func numericString(n pgtype.Numeric) string {
	v, err := n.Value()
	if err != nil {
		return ""
	}

	s, _ := v.(string)
	return s
}

type pageCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

var errInvalidCursor = errors.New("invalid cursor")

func (c pageCursor) encode() string {
	raw := strconv.FormatInt(c.CreatedAt.UnixMicro(), 10) + ":" + c.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(value string) (pageCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return pageCursor{}, errInvalidCursor
	}

	micros, id, ok := strings.Cut(string(raw), ":")
	if !ok {
		return pageCursor{}, errInvalidCursor
	}

	ts, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return pageCursor{}, errInvalidCursor
	}

	parsedID, err := uuid.Parse(id)
	if err != nil {
		return pageCursor{}, errInvalidCursor
	}

	return pageCursor{CreatedAt: time.UnixMicro(ts), ID: parsedID}, nil
}

func parseLimit(value string) (int, error) {
	if value == "" {
		return defaultLimit, nil
	}

	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > maxLimit {
		return 0, apperror.Validation("limit must be an integer between 1 and 100")
	}

	return limit, nil
}
