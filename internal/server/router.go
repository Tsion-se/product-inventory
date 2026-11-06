// Package server wires the HTTP router, middleware and routes together.
package server

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/example/product-inventory/internal/httpx"
	"github.com/example/product-inventory/internal/product"
)

// RequestTimeout bounds how long a request may run.
const RequestTimeout = 30 * time.Second

// Pinger is implemented by anything that can check database connectivity
// (for example *pgxpool.Pool).
type Pinger interface {
	Ping(ctx context.Context) error
}

// NewRouter builds the application router.
func NewRouter(svc *product.Service, db Pinger) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(httpx.JSONRecoverer)
	r.Use(middleware.Timeout(RequestTimeout))

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "The requested resource was not found.", nil)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed",
			"The HTTP method is not allowed for this resource.", nil)
	})

	r.Get("/healthz", healthHandler(db))

	r.Route("/api/v1", func(r chi.Router) {
		r.Mount("/products", product.NewHandler(svc).Routes())
	})

	return r
}

func healthHandler(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status":   "unavailable",
				"database": "down",
			})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{
			"status":   "ok",
			"database": "up",
		})
	}
}
