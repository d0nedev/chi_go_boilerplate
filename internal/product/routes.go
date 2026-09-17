package product

import (
	"github.com/d0nedev/chi_go_boilerplate/internal/platform/httpx"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func RegisterRoutes(
	r chi.Router,
	h *Handler,
	logger *slog.Logger,
	requireAuth func(http.Handler) http.Handler,
) {
	r.Route("/products", func(r chi.Router) {
		r.Get("/", httpx.Handle(logger, h.FindAll))
		r.With(requireAuth).Post("/", httpx.Handle(logger, h.CreateProduct))

		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", httpx.Handle(logger, h.FindByID))
			r.With(requireAuth).Put("/", httpx.Handle(logger, h.UpdateProduct))
			r.With(requireAuth).Delete("/", httpx.Handle(logger, h.DeleteProduct))
		})
	})
}
