package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/product-inventory/internal/product"
)

type pingFunc func(ctx context.Context) error

func (p pingFunc) Ping(ctx context.Context) error { return p(ctx) }

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	svc := product.NewService(newFakeRepo())
	return NewRouter(svc, pingFunc(func(context.Context) error { return nil }))
}

// do sends a request; a non-empty body is sent as application/json.
func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("response is not valid JSON: %v\nbody: %s", err, rec.Body.String())
	}
}

type errorResponse struct {
	Error   string            `json:"error"`
	Message string            `json:"message"`
	Details map[string]string `json:"details"`
}

func expectError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) errorResponse {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d\nbody: %s", rec.Code, status, rec.Body.String())
	}
	var e errorResponse
	decode(t, rec, &e)
	if e.Error != code {
		t.Fatalf("error code = %q, want %q (body: %s)", e.Error, code, rec.Body.String())
	}
	if e.Message == "" {
		t.Fatalf("error response has no message: %s", rec.Body.String())
	}
	return e
}

func createProduct(t *testing.T, h http.Handler, body string) map[string]any {
	t.Helper()
	rec := do(h, http.MethodPost, "/api/v1/products", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create failed: status %d, body %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	decode(t, rec, &got)
	return got
}

func productPath(p map[string]any) string {
	return fmt.Sprintf("/api/v1/products/%d", int64(p["id"].(float64)))
}

func TestCreateProduct(t *testing.T) {
	h := newTestHandler(t)

	rec := do(h, http.MethodPost, "/api/v1/products",
		`{"name":"  Widget  ","sku":"WID-001","description":"A widget","price":5}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201\nbody: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/api/v1/products/1" {
		t.Errorf("Location = %q, want /api/v1/products/1", loc)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}

	var got map[string]any
	decode(t, rec, &got)
	checks := map[string]any{
		"id":             float64(1),
		"name":           "Widget",
		"sku":            "WID-001",
		"description":    "A widget",
		"price":          float64(5),
		"stock_quantity": float64(0),
		"status":         "draft",
	}
	for k, want := range checks {
		if got[k] != want {
			t.Errorf("%s = %v (%T), want %v", k, got[k], got[k], want)
		}
	}
	if got["created_at"] == nil || got["updated_at"] == nil {
		t.Errorf("timestamps missing: %v", got)
	}
}

func TestCreateProductWithAllFields(t *testing.T) {
	h := newTestHandler(t)
	got := createProduct(t, h,
		`{"name":"Gadget","sku":"GAD-9","price":19.99,"stock_quantity":42,"status":"active"}`)
	if got["status"] != "active" || got["stock_quantity"] != float64(42) || got["price"] != 19.99 {
		t.Fatalf("unexpected product: %v", got)
	}
	if got["description"] != nil {
		t.Fatalf("description should be null, got %v", got["description"])
	}
}

func TestCreateProductValidation(t *testing.T) {
	h := newTestHandler(t)
	tests := []struct {
		name  string
		body  string
		field string
	}{
		{"empty name", `{"name":"   ","sku":"A-1","price":1}`, "name"},
		{"missing name", `{"sku":"A-1","price":1}`, "name"},
		{"missing sku", `{"name":"x","price":1}`, "sku"},
		{"lowercase sku", `{"name":"x","sku":"a-1","price":1}`, "sku"},
		{"sku with spaces", `{"name":"x","sku":"A 1","price":1}`, "sku"},
		{"negative price", `{"name":"x","sku":"A-1","price":-1}`, "price"},
		{"missing price", `{"name":"x","sku":"A-1"}`, "price"},
		{"too many decimals", `{"name":"x","sku":"A-1","price":1.234}`, "price"},
		{"invalid status", `{"name":"x","sku":"A-1","price":1,"status":"bogus"}`, "status"},
		{"negative stock", `{"name":"x","sku":"A-1","price":1,"stock_quantity":-3}`, "stock_quantity"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(h, http.MethodPost, "/api/v1/products", tt.body)
			e := expectError(t, rec, http.StatusBadRequest, "validation_failed")
			if _, ok := e.Details[tt.field]; !ok {
				t.Errorf("details %v missing key %q", e.Details, tt.field)
			}
		})
	}
}

func TestCreateProductBadJSON(t *testing.T) {
	h := newTestHandler(t)
	tests := []struct {
		name string
		body string
	}{
		{"truncated", `{"name":`},
		{"not json", `hello`},
		{"array", `[]`},
		{"trailing data", `{"name":"x","sku":"A-1","price":1}{}`},
		{"float stock", `{"name":"x","sku":"A-1","price":1,"stock_quantity":1.5}`},
		{"string name type", `{"name":123,"sku":"A-1","price":1}`},
		{"non numeric price", `{"name":"x","sku":"A-1","price":"abc"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(h, http.MethodPost, "/api/v1/products", tt.body)
			expectError(t, rec, http.StatusBadRequest, "invalid_json")
		})
	}

	// Empty body with a JSON content type.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/products", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	expectError(t, rec, http.StatusBadRequest, "invalid_json")
}

func TestCreateProductWrongContentType(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/products", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	expectError(t, rec, http.StatusUnsupportedMediaType, "unsupported_media_type")
}

func TestCreateProductDuplicateSKU(t *testing.T) {
	h := newTestHandler(t)
	createProduct(t, h, `{"name":"One","sku":"DUP-1","price":1}`)
	rec := do(h, http.MethodPost, "/api/v1/products", `{"name":"Two","sku":"DUP-1","price":2}`)
	expectError(t, rec, http.StatusConflict, "sku_conflict")
}

func TestGetProduct(t *testing.T) {
	h := newTestHandler(t)
	created := createProduct(t, h, `{"name":"One","sku":"ONE-1","price":3.5}`)

	rec := do(h, http.MethodGet, productPath(created), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got map[string]any
	decode(t, rec, &got)
	if got["sku"] != "ONE-1" || got["price"] != 3.5 {
		t.Fatalf("unexpected product: %v", got)
	}

	expectError(t, do(h, http.MethodGet, "/api/v1/products/999", ""), http.StatusNotFound, "not_found")
	for _, bad := range []string{"abc", "0", "-5", "1.5", "99999999999999999999"} {
		expectError(t, do(h, http.MethodGet, "/api/v1/products/"+bad, ""), http.StatusBadRequest, "invalid_id")
	}
}

func TestListProductsPagination(t *testing.T) {
	h := newTestHandler(t)
	for i := 1; i <= 15; i++ {
		createProduct(t, h, fmt.Sprintf(`{"name":"Item %d","sku":"P-%03d","price":%d}`, i, i, i))
	}

	type listResponse struct {
		Data       []map[string]any `json:"data"`
		Pagination struct {
			TotalItems int `json:"total_items"`
			Page       int `json:"page"`
			PageSize   int `json:"page_size"`
			TotalPages int `json:"total_pages"`
		} `json:"pagination"`
	}
	list := func(query string) (int, listResponse) {
		rec := do(h, http.MethodGet, "/api/v1/products"+query, "")
		var out listResponse
		if rec.Code == http.StatusOK {
			decode(t, rec, &out)
		}
		return rec.Code, out
	}

	// Defaults: page 1, page size 10.
	code, res := list("")
	if code != http.StatusOK || len(res.Data) != 10 || res.Pagination.TotalItems != 15 ||
		res.Pagination.Page != 1 || res.Pagination.PageSize != 10 || res.Pagination.TotalPages != 2 {
		t.Fatalf("default page: code=%d len=%d pagination=%+v", code, len(res.Data), res.Pagination)
	}

	// Page 2 of size 5 (newest first: ids 10..6).
	code, res = list("?page=2&page_size=5")
	if code != http.StatusOK || len(res.Data) != 5 || res.Pagination.TotalPages != 3 || res.Pagination.Page != 2 {
		t.Fatalf("page 2: code=%d len=%d pagination=%+v", code, len(res.Data), res.Pagination)
	}
	if res.Data[0]["id"] != float64(10) || res.Data[4]["id"] != float64(6) {
		t.Fatalf("unexpected ordering on page 2: first=%v last=%v", res.Data[0]["id"], res.Data[4]["id"])
	}

	// "limit" is accepted as an alias for page_size.
	code, res = list("?limit=3")
	if code != http.StatusOK || len(res.Data) != 3 || res.Pagination.PageSize != 3 || res.Pagination.TotalPages != 5 {
		t.Fatalf("limit alias: code=%d len=%d pagination=%+v", code, len(res.Data), res.Pagination)
	}

	// Oversized page_size is clamped to 100.
	code, res = list("?page_size=1000")
	if code != http.StatusOK || res.Pagination.PageSize != 100 {
		t.Fatalf("clamp: code=%d pagination=%+v", code, res.Pagination)
	}

	// A page past the end is empty but not an error, and data is [] not null.
	rec := do(h, http.MethodGet, "/api/v1/products?page=9", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Fatalf("past the end: code=%d body=%s", rec.Code, rec.Body.String())
	}

	// Invalid pagination parameters.
	for _, q := range []string{"?page=0", "?page=-1", "?page=abc", "?page_size=0", "?page_size=x", "?limit=-4"} {
		expectError(t, do(h, http.MethodGet, "/api/v1/products"+q, ""), http.StatusBadRequest, "validation_failed")
	}
}

func TestListProductsFilters(t *testing.T) {
	h := newTestHandler(t)
	createProduct(t, h, `{"name":"Red Widget","sku":"W-1","price":1,"status":"active","stock_quantity":3}`)
	createProduct(t, h, `{"name":"Blue Widget","sku":"W-2","price":2,"status":"draft","stock_quantity":10}`)
	createProduct(t, h, `{"name":"Green Gadget","sku":"G-1","price":3,"status":"active","stock_quantity":0}`)
	createProduct(t, h, `{"name":"100%_Pure","sku":"P-1","price":4,"status":"archived","stock_quantity":50}`)

	total := func(query string) int {
		t.Helper()
		rec := do(h, http.MethodGet, "/api/v1/products"+query, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status %d body %s", query, rec.Code, rec.Body.String())
		}
		var res struct {
			Pagination struct {
				TotalItems int `json:"total_items"`
			} `json:"pagination"`
		}
		decode(t, rec, &res)
		return res.Pagination.TotalItems
	}

	tests := []struct {
		query string
		want  int
	}{
		{"?status=active", 2},
		{"?status=draft", 1},
		{"?status=archived", 1},
		{"?q=widget", 2},
		{"?q=WIDGET&status=active", 1},
		{"?q=%25_", 1}, // literal "%_" must not act as a wildcard
		{"?q=%25", 1},
		{"?low_stock=true", 2},
		{"?low_stock=false", 4},
		{"?low_stock=true&status=active", 2},
	}
	for _, tt := range tests {
		if got := total(tt.query); got != tt.want {
			t.Errorf("GET %s: total_items = %d, want %d", tt.query, got, tt.want)
		}
	}

	expectError(t, do(h, http.MethodGet, "/api/v1/products?status=bogus", ""), http.StatusBadRequest, "validation_failed")
	expectError(t, do(h, http.MethodGet, "/api/v1/products?low_stock=maybe", ""), http.StatusBadRequest, "validation_failed")
}

func TestUpdateProduct(t *testing.T) {
	h := newTestHandler(t)
	first := createProduct(t, h, `{"name":"First","sku":"F-1","price":10,"description":"keep me","stock_quantity":5}`)
	createProduct(t, h, `{"name":"Second","sku":"S-1","price":20}`)
	path := productPath(first)

	for _, method := range []string{http.MethodPut, http.MethodPatch} {
		rec := do(h, method, path, `{"name":"Renamed","price":12.5,"status":"active"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d\nbody: %s", method, rec.Code, rec.Body.String())
		}
		var got map[string]any
		decode(t, rec, &got)
		if got["name"] != "Renamed" || got["price"] != 12.5 || got["status"] != "active" {
			t.Fatalf("%s: unexpected product: %v", method, got)
		}
		// Fields not in the body are preserved.
		if got["sku"] != "F-1" || got["description"] != "keep me" || got["stock_quantity"] != float64(5) {
			t.Fatalf("%s: omitted fields changed: %v", method, got)
		}
	}

	// Changing the SKU to a free value works; clearing the description works.
	rec := do(h, http.MethodPut, path, `{"sku":"F-2","description":""}`)
	var got map[string]any
	decode(t, rec, &got)
	if rec.Code != http.StatusOK || got["sku"] != "F-2" || got["description"] != nil {
		t.Fatalf("sku/description update: status=%d body=%v", rec.Code, got)
	}

	// Setting stock to zero must be honoured (not treated as "absent").
	rec = do(h, http.MethodPatch, path, `{"stock_quantity":0}`)
	decode(t, rec, &got)
	if rec.Code != http.StatusOK || got["stock_quantity"] != float64(0) {
		t.Fatalf("zero stock update: status=%d body=%v", rec.Code, got)
	}

	// Updating to another product's SKU conflicts.
	expectError(t, do(h, http.MethodPut, path, `{"sku":"S-1"}`), http.StatusConflict, "sku_conflict")

	// Round-tripping a product keeps working (unknown fields such as id are ignored).
	rec = do(h, http.MethodPut, path, `{"id":1,"name":"Round Trip","created_at":"2020-01-01T00:00:00Z"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("round trip: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateProductErrors(t *testing.T) {
	h := newTestHandler(t)
	created := createProduct(t, h, `{"name":"First","sku":"F-1","price":10}`)
	path := productPath(created)

	expectError(t, do(h, http.MethodPut, "/api/v1/products/999", `{"name":"x"}`), http.StatusNotFound, "not_found")
	expectError(t, do(h, http.MethodPut, "/api/v1/products/abc", `{"name":"x"}`), http.StatusBadRequest, "invalid_id")
	expectError(t, do(h, http.MethodPut, path, `{"name":"x"`), http.StatusBadRequest, "invalid_json")
	expectError(t, do(h, http.MethodPut, path, `{}`), http.StatusBadRequest, "validation_failed")

	tests := []struct {
		body  string
		field string
	}{
		{`{"name":""}`, "name"},
		{`{"price":-5}`, "price"},
		{`{"status":"nope"}`, "status"},
		{`{"stock_quantity":-1}`, "stock_quantity"},
		{`{"sku":"lower"}`, "sku"},
	}
	for _, tt := range tests {
		e := expectError(t, do(h, http.MethodPut, path, tt.body), http.StatusBadRequest, "validation_failed")
		if _, ok := e.Details[tt.field]; !ok {
			t.Errorf("body %s: details %v missing %q", tt.body, e.Details, tt.field)
		}
	}
}

func TestDeleteProduct(t *testing.T) {
	h := newTestHandler(t)
	created := createProduct(t, h, `{"name":"Gone","sku":"GONE-1","price":1}`)
	path := productPath(created)

	rec := do(h, http.MethodDelete, path, "")
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("delete: status=%d body=%q, want 204 and empty body", rec.Code, rec.Body.String())
	}
	expectError(t, do(h, http.MethodGet, path, ""), http.StatusNotFound, "not_found")
	expectError(t, do(h, http.MethodDelete, path, ""), http.StatusNotFound, "not_found")
	expectError(t, do(h, http.MethodDelete, "/api/v1/products/abc", ""), http.StatusBadRequest, "invalid_id")

	// A deleted SKU can be reused.
	createProduct(t, h, `{"name":"Back","sku":"GONE-1","price":1}`)
}

func TestUnknownRouteAndMethod(t *testing.T) {
	h := newTestHandler(t)
	expectError(t, do(h, http.MethodGet, "/nope", ""), http.StatusNotFound, "not_found")
	expectError(t, do(h, http.MethodDelete, "/api/v1/products", ""), http.StatusMethodNotAllowed, "method_not_allowed")
}

func TestHealthz(t *testing.T) {
	svc := product.NewService(newFakeRepo())

	up := NewRouter(svc, pingFunc(func(context.Context) error { return nil }))
	rec := do(up, http.MethodGet, "/healthz", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("healthy: status=%d body=%s", rec.Code, rec.Body.String())
	}

	down := NewRouter(svc, pingFunc(func(context.Context) error { return errors.New("db down") }))
	rec = do(down, http.MethodGet, "/healthz", "")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"database":"down"`) {
		t.Fatalf("unhealthy: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
