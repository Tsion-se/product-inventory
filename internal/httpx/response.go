// Package httpx contains small HTTP helpers shared across handlers:
// JSON responses, the uniform error envelope, request decoding and middleware.
package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// MaxBodyBytes caps request body size.
const MaxBodyBytes = 1 << 20 // 1 MiB

// ErrorBody is the uniform error schema used by every error response.
type ErrorBody struct {
	Error   string            `json:"error"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// WriteJSON writes v as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		slog.Error("encode response", "error", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal_error","message":"An unexpected error occurred."}` + "\n"))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// WriteError writes an error envelope.
func WriteError(w http.ResponseWriter, status int, code, message string, details map[string]string) {
	WriteJSON(w, status, ErrorBody{Error: code, Message: message, Details: details})
}

// DecodeJSON decodes a single JSON value from the request body into dst.
// Unknown fields are ignored so clients can round-trip a GET payload.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

// DecodeErrorMessage turns a decoding error into a human-readable message.
func DecodeErrorMessage(err error) string {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	var tooLarge *http.MaxBytesError

	switch {
	case errors.Is(err, io.EOF):
		return "Request body must not be empty."
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "Malformed JSON: unexpected end of input."
	case errors.As(err, &syntaxErr):
		return fmt.Sprintf("Malformed JSON at position %d.", syntaxErr.Offset)
	case errors.As(err, &typeErr):
		if typeErr.Field == "" {
			return "Request body must be a JSON object."
		}
		return fmt.Sprintf("Invalid value for field %q: expected %s, got JSON %s.", typeErr.Field, typeErr.Type, typeErr.Value)
	case errors.As(err, &tooLarge):
		return fmt.Sprintf("Request body must not exceed %d bytes.", tooLarge.Limit)
	default:
		return "Invalid JSON: " + err.Error()
	}
}
