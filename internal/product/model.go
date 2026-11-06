// Package product contains the product domain: models, validation (service),
// persistence (repository) and HTTP handlers.
package product

import (
	"encoding/json"
	"errors"
	"time"
)

// Allowed product statuses.
const (
	StatusDraft    = "draft"
	StatusActive   = "active"
	StatusArchived = "archived"
)

// Pagination defaults and limits.
const (
	DefaultPageSize = 10
	MaxPageSize     = 100
)

// Product is the API representation of a product.
type Product struct {
	ID            int64       `json:"id"`
	Name          string      `json:"name"`
	SKU           string      `json:"sku"`
	Description   *string     `json:"description"`
	Price         json.Number `json:"price"`
	StockQuantity int32       `json:"stock_quantity"`
	Status        string      `json:"status"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

// Input is the request payload for creating and updating products.
// Pointers (and an empty Price) distinguish "absent" from zero values.
type Input struct {
	Name          *string     `json:"name"`
	SKU           *string     `json:"sku"`
	Description   *string     `json:"description"`
	Price         json.Number `json:"price"`
	StockQuantity *int64      `json:"stock_quantity"`
	Status        *string     `json:"status"`
}

// NewProduct is a fully validated product ready to be persisted.
// Price is a decimal string with exactly two fraction digits, e.g. "19.99".
type NewProduct struct {
	Name          string
	SKU           string
	Description   *string
	Price         string
	StockQuantity int32
	Status        string
}

// Filter narrows List and Count results.
type Filter struct {
	Status   *string // exact status match
	Search   *string // already LIKE-escaped; matched with ILIKE '%value%'
	LowStock bool    // stock_quantity <= 5
}

// ListParams are the (unvalidated) listing parameters from the HTTP layer.
type ListParams struct {
	Page     int
	PageSize int
	Status   string
	Search   string
	LowStock bool
}

// Pagination is the pagination metadata of a list response.
type Pagination struct {
	TotalItems int64 `json:"total_items"`
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalPages int   `json:"total_pages"`
}

// ListResult is the envelope returned by GET /products.
type ListResult struct {
	Data       []Product  `json:"data"`
	Pagination Pagination `json:"pagination"`
}

// Domain errors, mapped to HTTP statuses by the handler.
var (
	ErrNotFound    = errors.New("product not found")
	ErrSKUConflict = errors.New("a product with this SKU already exists")
)

// ValidationError carries per-field validation messages.
type ValidationError struct {
	Details map[string]string
}

func (e *ValidationError) Error() string { return "validation failed" }

func validStatus(s string) bool {
	return s == StatusDraft || s == StatusActive || s == StatusArchived
}
