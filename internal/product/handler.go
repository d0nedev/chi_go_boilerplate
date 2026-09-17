package product

import (
	"chi-product-api/internal/platform/apperror"
	"chi-product-api/internal/platform/httpx"
	"net/http"
	"strings"
	"uuid"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{
		service: service,
	}
}

func (h *Handler) FindAll(w http.ResponseWriter, r *http.Request) error {
	query := r.URL.Query()

	limit, err := parseLimit(query.Get("limit"))
	if err != nil {
		return err
	}

	var after *pageCursor
	if value := query.Get("cursor"); value != "" {
		cursor, err := decodeCursor(value)
		if err != nil {
			return apperror.Validation("invalid cursor")
		}
		after = &cursor
	}

	products, next, err := h.service.List(r.Context(), limit, after)
	if err != nil {
		return err
	}

	response := ListProductsResponse{
		Data: make([]ProductResponse, 0, len(products)),
	}

	for _, product := range products {
		response.Data = append(response.Data, toProductResponse(product))
	}

	if next != nil {
		encoded := next.encode()
		response.NextCursor = &encoded
	}

	return httpx.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) FindByID(w http.ResponseWriter, r *http.Request) error {
	id, err := productID(r)
	if err != nil {
		return err
	}

	product, err := h.service.FindByID(r.Context(), id)
	if err != nil {
		return err
	}

	return httpx.WriteJSON(w, http.StatusOK, toProductResponse(product))
}

func (h *Handler) CreateProduct(w http.ResponseWriter, r *http.Request) error {
	req, err := decodeProductRequest(w, r)
	if err != nil {
		return err
	}

	product, err := h.service.Create(r.Context(), req)
	if err != nil {
		return err
	}

	return httpx.WriteJSON(w, http.StatusCreated, toProductResponse(product))
}

func (h *Handler) UpdateProduct(w http.ResponseWriter, r *http.Request) error {
	id, err := productID(r)
	if err != nil {
		return err
	}

	req, err := decodeProductRequest(w, r)
	if err != nil {
		return err
	}

	product, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		return err
	}

	return httpx.WriteJSON(w, http.StatusOK, toProductResponse(product))
}

func (h *Handler) DeleteProduct(w http.ResponseWriter, r *http.Request) error {
	id, err := productID(r)
	if err != nil {
		return err
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)

	return nil
}

func productID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.UUID{}, apperror.Validation("invalid product id")
	}

	return id, nil
}

func decodeProductRequest(w http.ResponseWriter, r *http.Request) (ProductRequest, error) {
	var req ProductRequest

	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return req, err
	}

	if err := req.Validate(); err != nil {
		return req, err
	}

	req.Name = strings.TrimSpace(req.Name)

	return req, nil
}
