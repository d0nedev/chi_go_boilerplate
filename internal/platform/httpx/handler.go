package httpx

import (
	"github.com/d0nedev/chi_go_boilerplate/internal/platform/logging"
	"log/slog"
	"net/http"
)

type HandlerFunc func(http.ResponseWriter, *http.Request) error

func Handle(
	logger *slog.Logger,
	h HandlerFunc,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			logging.LogError(logger, r, err)
			WriteError(w, err)
		}
	}
}
