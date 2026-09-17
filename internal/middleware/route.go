package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

// RouteTag names the otelhttp server span and labels its metrics with the chi
// route pattern. The pattern is only complete after routing, so it runs post-handler.
func RouteTag(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)

		rctx := chi.RouteContext(r.Context())
		if rctx == nil {
			return
		}

		pattern := rctx.RoutePattern()
		if pattern == "" {
			return
		}

		route := semconv.HTTPRoute(pattern)

		span := trace.SpanFromContext(r.Context())
		span.SetName(r.Method + " " + pattern)
		span.SetAttributes(route)

		if labeler, ok := otelhttp.LabelerFromContext(r.Context()); ok {
			labeler.Add(route)
		}
	})
}
