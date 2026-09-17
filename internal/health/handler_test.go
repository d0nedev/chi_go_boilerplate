package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadyFailsWhileDraining(t *testing.T) {
	h := NewHandler(nil)
	h.StartDraining()

	rec := httptest.NewRecorder()
	h.Ready(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d", rec.Code)
	}
}
