package product

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	db "chi-product-api/internal/database/sqlc"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace/noop"
)

// newIntegrationPool applies all up migrations into a throwaway schema so the
// test never touches existing tables in TEST_DATABASE_URL.
func newIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx := context.Background()
	schema := fmt.Sprintf("it_%d", time.Now().UnixNano())

	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close(context.Background())
	})

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	files, err := filepath.Glob("../../db/migrations/*.up.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("migrations not found: %v", err)
	}
	sort.Strings(files)

	for _, f := range files {
		sql, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", f, err)
		}
	}

	return pool
}

func TestIntegrationProductLifecycle(t *testing.T) {
	pool := newIntegrationPool(t)

	service := NewService(db.New(pool), noop.NewTracerProvider().Tracer("test"))
	r := chi.NewRouter()
	RegisterRoutes(r, NewHandler(service), slog.New(slog.NewTextHandler(io.Discard, nil)),
		func(next http.Handler) http.Handler { return next })

	call := func(method, path, body string) (int, []byte) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec.Code, rec.Body.Bytes()
	}

	var ids []string
	for i := range 3 {
		code, body := call(http.MethodPost, "/products/", fmt.Sprintf(`{"name":"p%d","price":"%d.50"}`, i, i))
		if code != http.StatusCreated {
			t.Fatalf("create: %d %s", code, body)
		}
		var p ProductResponse
		_ = json.Unmarshal(body, &p)
		ids = append(ids, p.ID)
	}

	// Page 1: newest two, with cursor.
	code, body := call(http.MethodGet, "/products/?limit=2", "")
	var page ListProductsResponse
	if err := json.Unmarshal(body, &page); err != nil || code != http.StatusOK {
		t.Fatalf("list: %d %s", code, body)
	}
	if len(page.Data) != 2 || page.NextCursor == nil {
		t.Fatalf("page1 = %s", body)
	}
	if page.Data[0].ID != ids[2] || page.Data[1].ID != ids[1] {
		t.Errorf("page1 order = %s", body)
	}

	// Page 2: remaining one, no cursor.
	_, body = call(http.MethodGet, "/products/?limit=2&cursor="+*page.NextCursor, "")
	page = ListProductsResponse{}
	_ = json.Unmarshal(body, &page)
	if len(page.Data) != 1 || page.Data[0].ID != ids[0] || page.NextCursor != nil {
		t.Fatalf("page2 = %s", body)
	}

	code, body = call(http.MethodPut, "/products/"+ids[0]+"/", `{"name":"renamed","price":"9999999999999.99"}`)
	if code != http.StatusOK || !strings.Contains(string(body), `"price":"9999999999999.99"`) {
		t.Fatalf("update: %d %s", code, body)
	}

	if code, body = call(http.MethodDelete, "/products/"+ids[0]+"/", ""); code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", code, body)
	}
	if code, _ = call(http.MethodGet, "/products/"+ids[0]+"/", ""); code != http.StatusNotFound {
		t.Errorf("get deleted: %d", code)
	}
	if code, _ = call(http.MethodDelete, "/products/"+ids[0]+"/", ""); code != http.StatusNotFound {
		t.Errorf("delete twice: %d", code)
	}
}

func TestIntegrationDatabaseConstraints(t *testing.T) {
	pool := newIntegrationPool(t)
	q := db.New(pool)
	ctx := context.Background()

	var negative pgtype.Numeric
	_ = negative.Scan("-1")

	if _, err := q.CreateProduct(ctx, db.CreateProductParams{Name: "x", Price: negative}); err == nil {
		t.Error("negative price accepted by database")
	}

	var price pgtype.Numeric
	_ = price.Scan("1")

	if _, err := q.CreateProduct(ctx, db.CreateProductParams{Name: "   ", Price: price}); err == nil {
		t.Error("blank name accepted by database")
	}
}
