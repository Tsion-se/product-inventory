package product

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/example/product-inventory/internal/httpx"
)

// Handler exposes the product service over HTTP.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes returns the router to be mounted at /api/v1/products.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.With(httpx.RequireJSON).Post("/", h.create)
	r.Get("/{id}", h.get)
	r.With(httpx.RequireJSON).Put("/{id}", h.update)
	r.With(httpx.RequireJSON).Patch("/{id}", h.update)
	r.Delete("/{id}", h.delete)
	return r
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in Input
	if !decodeBody(w, r, &in) {
		return
	}
	p, err := h.svc.Create(r.Context(), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/products/%d", p.ID))
	httpx.WriteJSON(w, http.StatusCreated, p)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	params, err := parseListParams(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	res, err := h.svc.List(r.Context(), params)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

// update serves both PUT and PATCH: fields present in the body are updated,
// absent fields keep their current value.
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var in Input
	if !decodeBody(w, r, &in) {
		return
	}
	p, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// fail maps domain and infrastructure errors onto HTTP error responses.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var ve *ValidationError
	switch {
	case errors.As(err, &ve):
		httpx.WriteError(w, http.StatusBadRequest, "validation_failed",
			"One or more fields are invalid.", ve.Details)
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Product not found.", nil)
	case errors.Is(err, ErrSKUConflict):
		httpx.WriteError(w, http.StatusConflict, "sku_conflict",
			"A product with this SKU already exists.", map[string]string{"sku": "already exists"})
	case errors.Is(err, context.DeadlineExceeded):
		httpx.WriteError(w, http.StatusGatewayTimeout, "timeout", "The request timed out.", nil)
	default:
		slog.Error("internal error",
			"request_id", middleware.GetReqID(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error",
			"An unexpected error occurred.", nil)
	}
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := httpx.DecodeJSON(w, r, dst); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", httpx.DecodeErrorMessage(err), nil)
		return false
	}
	return true
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id",
			"Product ID must be a positive integer.", map[string]string{"id": "must be a positive integer"})
		return 0, false
	}
	return id, true
}

// parseListParams parses query-string types; range and enum validation happens
// in the service.
func parseListParams(r *http.Request) (ListParams, error) {
	q := r.URL.Query()
	details := map[string]string{}
	params := ListParams{Page: 1, PageSize: DefaultPageSize}

	if v := strings.TrimSpace(q.Get("page")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			details["page"] = "must be an integer"
		} else {
			params.Page = n
		}
	}

	sizeKey := "page_size"
	sizeVal := strings.TrimSpace(q.Get("page_size"))
	if sizeVal == "" {
		sizeKey = "limit"
		sizeVal = strings.TrimSpace(q.Get("limit"))
	}
	if sizeVal != "" {
		n, err := strconv.Atoi(sizeVal)
		if err != nil {
			details[sizeKey] = "must be an integer"
		} else {
			params.PageSize = n
		}
	}

	params.Status = q.Get("status")
	params.Search = q.Get("q")

	if v := strings.TrimSpace(q.Get("low_stock")); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			details["low_stock"] = "must be true or false"
		} else {
			params.LowStock = b
		}
	}

	if len(details) > 0 {
		return params, &ValidationError{Details: details}
	}
	return params, nil
}
