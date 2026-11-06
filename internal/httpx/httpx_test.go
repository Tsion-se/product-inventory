package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSONRecovererReturnsErrorEnvelope(t *testing.T) {
	h := JSONRecoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, rec.Body.String())
	}
	if body.Error != "internal_error" || body.Message == "" {
		t.Fatalf("unexpected error body: %+v", body)
	}
	if strings.Contains(rec.Body.String(), "boom") {
		t.Fatal("panic value must not leak to the client")
	}
}

func TestRequireJSON(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := RequireJSON(next)

	tests := []struct {
		contentType string
		want        int
	}{
		{"application/json", http.StatusOK},
		{"application/json; charset=utf-8", http.StatusOK},
		{"text/plain", http.StatusUnsupportedMediaType},
		{"", http.StatusUnsupportedMediaType},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
		if tt.contentType != "" {
			req.Header.Set("Content-Type", tt.contentType)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tt.want {
			t.Errorf("Content-Type %q: status = %d, want %d", tt.contentType, rec.Code, tt.want)
		}
	}
}

func TestDecodeJSONRejectsTrailingData(t *testing.T) {
	var dst struct {
		A int `json:"a"`
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":1}{"a":2}`))
	if err := DecodeJSON(httptest.NewRecorder(), req, &dst); err == nil {
		t.Fatal("expected an error for trailing JSON")
	}

	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":1}`))
	if err := DecodeJSON(httptest.NewRecorder(), req, &dst); err != nil || dst.A != 1 {
		t.Fatalf("unexpected result: err=%v a=%d", err, dst.A)
	}
}
