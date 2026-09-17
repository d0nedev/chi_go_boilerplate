package app

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/d0nedev/chi_go_boilerplate/internal/platform/config"
	"github.com/d0nedev/chi_go_boilerplate/internal/platform/health"

	"go.opentelemetry.io/otel/trace/noop"
)

func testRouter(t *testing.T, logs *bytes.Buffer) (http.Handler, *health.Handler) {
	t.Helper()

	cfg := &config.Config{
		Auth:      config.AuthConfig{APIKeys: []string{"secret"}},
		RateLimit: config.RateLimitConfig{RequestsPerMinute: 3},
	}
	logger := slog.New(slog.NewJSONHandler(logs, nil))

	// Requests in these tests never reach the database, so a nil pool is enough.
	probes := health.NewHandler(nil)
	routes := modules(cfg, logger, nil, noop.NewTracerProvider())

	return newRouter(cfg, logger, probes, routes...), probes
}

func serve(h http.Handler, method, path string, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.RemoteAddr = "203.0.113.10:1234"
	for k, v := range header {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRouterErrorsAreJSON(t *testing.T) {
	h, _ := testRouter(t, &bytes.Buffer{})

	tests := []struct {
		method, path string
		status       int
		code         string
	}{
		{http.MethodGet, "/nope", http.StatusNotFound, "NOT_FOUND"},
		{http.MethodPatch, "/api/v1/products/", http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
		{http.MethodPost, "/api/v1/products/", http.StatusUnauthorized, "UNAUTHORIZED"},
		{http.MethodGet, "/api/v1/products/not-a-uuid", http.StatusBadRequest, "VALIDATION_ERROR"},
	}

	for _, tt := range tests {
		rec := serve(h, tt.method, tt.path, nil)

		var body struct {
			Error struct{ Code string } `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("%s %s: non-JSON body %q", tt.method, tt.path, rec.Body)
			continue
		}
		if rec.Code != tt.status || body.Error.Code != tt.code {
			t.Errorf("%s %s: got %d %s, want %d %s", tt.method, tt.path, rec.Code, body.Error.Code, tt.status, tt.code)
		}
	}
}

func TestRouterSecurityAndRequestIDHeaders(t *testing.T) {
	h, _ := testRouter(t, &bytes.Buffer{})

	rec := serve(h, http.MethodGet, "/health", http.Header{"X-Request-Id": {"bad id with spaces"}})

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", got)
	}
	if got := rec.Header().Get("X-Request-ID"); got == "" || strings.Contains(got, " ") {
		t.Errorf("invalid client request ID should be replaced, got %q", got)
	}
}

func TestRouterRateLimitsAPIButNotProbes(t *testing.T) {
	h, _ := testRouter(t, &bytes.Buffer{})

	for range 3 {
		serve(h, http.MethodGet, "/api/v1/products/not-a-uuid", nil)
	}
	if rec := serve(h, http.MethodGet, "/api/v1/products/not-a-uuid", nil); rec.Code != http.StatusTooManyRequests {
		t.Errorf("4th API request: %d", rec.Code)
	}
	for range 5 {
		if rec := serve(h, http.MethodGet, "/health", nil); rec.Code != http.StatusOK {
			t.Fatalf("probe rate limited: %d", rec.Code)
		}
	}
}

func TestRouterProbeLogging(t *testing.T) {
	var logs bytes.Buffer
	h, probes := testRouter(t, &logs)

	serve(h, http.MethodGet, "/health", nil)
	if logs.Len() != 0 {
		t.Fatalf("successful probe should not be logged: %s", logs.String())
	}

	probes.StartDraining()
	serve(h, http.MethodGet, "/ready", nil)

	if !strings.Contains(logs.String(), `"level":"WARN"`) || !strings.Contains(logs.String(), `"status":503`) {
		t.Errorf("failed probe should be logged at WARN: %s", logs.String())
	}
}
