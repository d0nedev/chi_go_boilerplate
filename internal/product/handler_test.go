package product

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	db "chi-product-api/internal/database/sqlc"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel/trace/noop"
)

// fakeDB echoes query args back as the returned product row.
type fakeDB struct{}

func (fakeDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("not implemented")
}

func (fakeDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}

func (fakeDB) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	p := db.Product{
		ID:        pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
		CreatedAt: pgtype.Timestamptz{Time: time.Unix(0, 0), Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: time.Unix(0, 0), Valid: true},
	}

	switch len(args) {
	case 2: // CreateProduct(name, price)
		p.Name = args[0].(string)
		p.Price = args[1].(pgtype.Numeric)
	case 3: // UpdateProduct(id, name, price)
		p.Name = args[1].(string)
		p.Price = args[2].(pgtype.Numeric)
	}

	return fakeRow{p}
}

type fakeRow struct{ p db.Product }

func (r fakeRow) Scan(dest ...any) error {
	*dest[0].(*pgtype.UUID) = r.p.ID
	*dest[1].(*string) = r.p.Name
	*dest[2].(*pgtype.Numeric) = r.p.Price
	*dest[3].(*pgtype.Timestamptz) = r.p.CreatedAt
	*dest[4].(*pgtype.Timestamptz) = r.p.UpdatedAt
	return nil
}

func newTestRouter() http.Handler {
	service := NewService(db.New(fakeDB{}), noop.NewTracerProvider().Tracer("test"))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	r := chi.NewRouter()
	RegisterRoutes(r, NewHandler(service), logger, func(next http.Handler) http.Handler { return next })
	return r
}

func do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, req)
	return rec
}

func TestCreateProductReturnsDecimalPrice(t *testing.T) {
	rec := do(t, http.MethodPost, "/products/", `{"name":"Lenovo vPro","price":"13000000.00"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var got ProductResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Price != "13000000.00" {
		t.Errorf("price = %q, want %q", got.Price, "13000000.00")
	}
	if got.CreatedAt != "1970-01-01T00:00:00Z" {
		t.Errorf("created_at = %q, want RFC 3339", got.CreatedAt)
	}
}

func TestWriteEndpointsRejectInvalidInput(t *testing.T) {
	const id = "d90ace71-4deb-4857-966b-2b97a64e6aed"

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		wantCode int
		wantText string
	}{
		{"create invalid price", http.MethodPost, "/products/", `{"name":"x","price":"abc"}`, 400, "price must be"},
		{"create negative price", http.MethodPost, "/products/", `{"name":"x","price":"-1.00"}`, 400, "price must be"},
		{"create numeric price", http.MethodPost, "/products/", `{"name":"x","price":12}`, 400, "invalid JSON body"},
		{"create blank name", http.MethodPost, "/products/", `{"name":"  ","price":"1.00"}`, 400, "name is required"},
		{"create unknown field", http.MethodPost, "/products/", `{"name":"x","price":"1.00","stock":1}`, 400, "invalid JSON body"},
		{"create empty body", http.MethodPost, "/products/", ``, 400, "request body is required"},
		{"create too large", http.MethodPost, "/products/", `{"name":"` + strings.Repeat("a", 1<<20) + `","price":"1"}`, 413, "REQUEST_BODY_TOO_LARGE"},
		{"update blank name", http.MethodPut, "/products/" + id + "/", `{"name":"","price":"1.00"}`, 400, "name is required"},
		{"update bad id", http.MethodPut, "/products/nope/", `{"name":"x","price":"1.00"}`, 400, "invalid product id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, tt.method, tt.path, tt.body)

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.wantCode, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), tt.wantText) {
				t.Errorf("body = %s, want substring %q", rec.Body, tt.wantText)
			}
		})
	}
}

func TestListRejectsInvalidQuery(t *testing.T) {
	for _, path := range []string{"/products/?limit=0", "/products/?limit=101", "/products/?limit=x", "/products/?cursor=not-a-cursor!"} {
		rec := do(t, http.MethodGet, path, "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, body = %s", path, rec.Code, rec.Body)
		}
	}
}

func TestUpdateProductWritesSingleJSONBody(t *testing.T) {
	rec := do(t, http.MethodPut, "/products/d90ace71-4deb-4857-966b-2b97a64e6aed/",
		`{"name":"MacBook Pro M5","price":"27000000.00"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	dec := json.NewDecoder(rec.Body)
	var got ProductResponse
	if err := dec.Decode(&got); err != nil {
		t.Fatal(err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		t.Errorf("expected single JSON object, extra decode err = %v", err)
	}
	if got.Price != "27000000.00" {
		t.Errorf("price = %q", got.Price)
	}
}

func TestPanicRouteRemoved(t *testing.T) {
	rec := do(t, http.MethodGet, "/products/panic", "")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (treated as invalid id), body = %s", rec.Code, rec.Body)
	}
}
