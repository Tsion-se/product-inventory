package httpx

import (
	"log/slog"
	"mime"
	"net/http"
	"runtime/debug"

	"github.com/go-chi/chi/v5/middleware"
)

// RequireJSON rejects requests whose Content-Type is not application/json.
// Apply it to routes that read a JSON body (POST, PUT, PATCH).
func RequireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			WriteError(w, http.StatusUnsupportedMediaType, "unsupported_media_type",
				"Content-Type must be application/json.", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// JSONRecoverer recovers from panics and answers with the uniform JSON error
// envelope. It sits inside chi's middleware.Recoverer, which remains the
// outermost safety net.
func JSONRecoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				slog.Error("panic recovered",
					"request_id", middleware.GetReqID(r.Context()),
					"panic", rec,
					"stack", string(debug.Stack()))
				WriteError(w, http.StatusInternalServerError, "internal_error",
					"An unexpected error occurred.", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
