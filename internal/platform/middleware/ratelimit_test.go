package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func limited(trustedProxies []string, rpm int) http.Handler {
	return ClientIP(trustedProxies)(RateLimit(rpm)(ok))
}

func request(h http.Handler, remoteAddr, xff string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRateLimitPerRemoteAddr(t *testing.T) {
	h := limited(nil, 1)

	if rec := request(h, "203.0.113.1:1000", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("first request: %d", rec.Code)
	}

	rec := request(h, "203.0.113.1:2000", "")
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "RATE_LIMITED") {
		t.Fatalf("second request same IP: %d %s", rec.Code, rec.Body)
	}

	if rec := request(h, "203.0.113.2:1000", ""); rec.Code != http.StatusNoContent {
		t.Errorf("other IP should have its own bucket: %d", rec.Code)
	}
}

func TestRateLimitIgnoresXFFWithoutTrustedProxies(t *testing.T) {
	h := limited(nil, 1)

	request(h, "203.0.113.1:1000", "198.51.100.1")

	// Spoofed XFF must not grant a fresh bucket.
	if rec := request(h, "203.0.113.1:1000", "198.51.100.2"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("spoofed XFF bypassed limit: %d", rec.Code)
	}
}

func TestRateLimitBehindTrustedProxy(t *testing.T) {
	h := limited([]string{"10.0.0.0/8"}, 1)

	if rec := request(h, "10.0.0.5:1000", "198.51.100.1"); rec.Code != http.StatusNoContent {
		t.Fatalf("client A: %d", rec.Code)
	}
	if rec := request(h, "10.0.0.5:1000", "198.51.100.2"); rec.Code != http.StatusNoContent {
		t.Errorf("client B behind same proxy should have own bucket: %d", rec.Code)
	}
	if rec := request(h, "10.0.0.5:1000", "198.51.100.1"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("client A second request: %d", rec.Code)
	}
}

func TestXFFFromUntrustedPeerIsIgnored(t *testing.T) {
	h := limited([]string{"10.0.0.0/8"}, 1)

	request(h, "203.0.113.9:1000", "198.51.100.1")

	// A direct client rotating forged XFF values must stay in its own peer bucket.
	if rec := request(h, "203.0.113.9:1000", "198.51.100.2"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("forged XFF from untrusted peer bypassed limit: %d", rec.Code)
	}
}

func TestRateLimitRejectsUnresolvedClientIP(t *testing.T) {
	// Behind a trusted proxy with no XFF, ClientIPFromXFF sets no IP.
	h := limited([]string{"10.0.0.0/8"}, 100)

	rec := request(h, "10.0.0.5:1000", "")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "CLIENT_IP_UNRESOLVED") {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestClientIPUsedForLogging(t *testing.T) {
	var got string
	h := ClientIP([]string{"10.0.0.0/8"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = chimiddleware.GetClientIP(r.Context())
	}))

	request(h, "10.0.0.5:1000", "198.51.100.7")

	if got != "198.51.100.7" {
		t.Errorf("client IP = %q", got)
	}
}
