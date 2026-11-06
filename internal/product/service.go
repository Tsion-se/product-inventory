package product

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	maxNameLen        = 255
	maxSKULen         = 64
	maxDescriptionLen = 10000
	maxSearchLen      = 100
	maxPage           = 1_000_000
)

var (
	// Uppercase alphanumeric segments, optionally separated by single hyphens.
	skuPattern = regexp.MustCompile(`^[A-Z0-9]+(-[A-Z0-9]+)*$`)
	// Non-negative decimal, up to 8 integer digits and 2 fraction digits (NUMERIC(10,2)).
	pricePattern = regexp.MustCompile(`^[0-9]{1,8}(\.[0-9]{1,2})?$`)
	// Escapes LIKE/ILIKE wildcards so user input is matched literally.
	likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
)

// Service holds the business rules: validation and orchestration.
type Service struct {
	repo Repository
}

// NewService creates a Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create validates the input and creates a product.
func (s *Service) Create(ctx context.Context, in Input) (Product, error) {
	p, details := validateInput(in, true)
	if len(details) > 0 {
		return Product{}, &ValidationError{Details: details}
	}

	np := NewProduct{
		Name:          *p.name,
		SKU:           *p.sku,
		Description:   p.description,
		Price:         *p.price,
		StockQuantity: 0,
		Status:        StatusDraft,
	}
	if p.stock != nil {
		np.StockQuantity = *p.stock
	}
	if p.status != nil {
		np.Status = *p.status
	}
	return s.repo.Create(ctx, np)
}

// Get returns a product by ID.
func (s *Service) Get(ctx context.Context, id int64) (Product, error) {
	return s.repo.GetByID(ctx, id)
}

// Update applies the provided fields to an existing product (fields that are
// absent from the input keep their current value).
func (s *Service) Update(ctx context.Context, id int64, in Input) (Product, error) {
	p, details := validateInput(in, false)
	if len(details) == 0 && p.empty() {
		details = map[string]string{
			"body": "at least one of name, sku, description, price, stock_quantity, status must be provided",
		}
	}
	if len(details) > 0 {
		return Product{}, &ValidationError{Details: details}
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Product{}, err
	}

	np := NewProduct{
		Name:          existing.Name,
		SKU:           existing.SKU,
		Description:   existing.Description,
		Price:         existing.Price.String(),
		StockQuantity: existing.StockQuantity,
		Status:        existing.Status,
	}
	if p.name != nil {
		np.Name = *p.name
	}
	if p.sku != nil {
		np.SKU = *p.sku
	}
	if p.descriptionSet {
		np.Description = p.description // nil clears the description
	}
	if p.price != nil {
		np.Price = *p.price
	}
	if p.stock != nil {
		np.StockQuantity = *p.stock
	}
	if p.status != nil {
		np.Status = *p.status
	}
	return s.repo.Update(ctx, id, np)
}

// Delete removes a product, returning ErrNotFound if it does not exist.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

// List returns a page of products plus pagination metadata.
func (s *Service) List(ctx context.Context, params ListParams) (ListResult, error) {
	details := map[string]string{}

	if params.Page < 1 || params.Page > maxPage {
		details["page"] = fmt.Sprintf("must be an integer between 1 and %d", maxPage)
	}
	if params.PageSize < 1 {
		details["page_size"] = fmt.Sprintf("must be an integer between 1 and %d", MaxPageSize)
	}
	pageSize := params.PageSize
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}

	filter := Filter{LowStock: params.LowStock}
	if status := strings.TrimSpace(params.Status); status != "" {
		if !validStatus(status) {
			details["status"] = "must be one of: draft, active, archived"
		} else {
			filter.Status = &status
		}
	}
	if search := strings.TrimSpace(params.Search); search != "" {
		if utf8.RuneCountInString(search) > maxSearchLen {
			details["q"] = fmt.Sprintf("must be at most %d characters", maxSearchLen)
		} else {
			escaped := likeEscaper.Replace(search)
			filter.Search = &escaped
		}
	}
	if len(details) > 0 {
		return ListResult{}, &ValidationError{Details: details}
	}

	total, err := s.repo.Count(ctx, filter)
	if err != nil {
		return ListResult{}, err
	}
	offset := (params.Page - 1) * pageSize
	items, err := s.repo.List(ctx, filter, int32(pageSize), int32(offset))
	if err != nil {
		return ListResult{}, err
	}
	if items == nil {
		items = []Product{}
	}

	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	return ListResult{
		Data: items,
		Pagination: Pagination{
			TotalItems: total,
			Page:       params.Page,
			PageSize:   pageSize,
			TotalPages: totalPages,
		},
	}, nil
}

// parsed holds validated, normalised input fields. A nil pointer means the
// field was not supplied.
type parsed struct {
	name           *string
	sku            *string
	description    *string
	descriptionSet bool
	price          *string
	stock          *int32
	status         *string
}

func (p parsed) empty() bool {
	return p.name == nil && p.sku == nil && !p.descriptionSet &&
		p.price == nil && p.stock == nil && p.status == nil
}

// validateInput validates the supplied fields. When create is true, name, sku
// and price are required.
func validateInput(in Input, create bool) (parsed, map[string]string) {
	var p parsed
	details := map[string]string{}

	if in.Name != nil {
		if v, msg := validateName(*in.Name); msg != "" {
			details["name"] = msg
		} else {
			p.name = &v
		}
	} else if create {
		details["name"] = "is required"
	}

	if in.SKU != nil {
		if v, msg := validateSKU(*in.SKU); msg != "" {
			details["sku"] = msg
		} else {
			p.sku = &v
		}
	} else if create {
		details["sku"] = "is required"
	}

	if in.Price != "" {
		if v, msg := validatePrice(in.Price); msg != "" {
			details["price"] = msg
		} else {
			p.price = &v
		}
	} else if create {
		details["price"] = "is required"
	}

	if in.Description != nil {
		if v, msg := validateDescription(*in.Description); msg != "" {
			details["description"] = msg
		} else {
			p.description = v
			p.descriptionSet = true
		}
	}

	if in.StockQuantity != nil {
		if v, msg := validateStock(*in.StockQuantity); msg != "" {
			details["stock_quantity"] = msg
		} else {
			p.stock = &v
		}
	}

	if in.Status != nil {
		if !validStatus(*in.Status) {
			details["status"] = "must be one of: draft, active, archived"
		} else {
			v := *in.Status
			p.status = &v
		}
	}

	return p, details
}

func validateName(raw string) (string, string) {
	name := strings.TrimSpace(raw)
	switch {
	case name == "":
		return "", "must not be empty"
	case utf8.RuneCountInString(name) > maxNameLen:
		return "", fmt.Sprintf("must be at most %d characters", maxNameLen)
	}
	return name, ""
}

func validateSKU(raw string) (string, string) {
	sku := strings.TrimSpace(raw)
	switch {
	case sku == "":
		return "", "must not be empty"
	case len(sku) > maxSKULen:
		return "", fmt.Sprintf("must be at most %d characters", maxSKULen)
	case !skuPattern.MatchString(sku):
		return "", "must contain only uppercase letters and digits, optionally separated by single hyphens (e.g. LAP-001)"
	}
	return sku, ""
}

// validateDescription returns nil for a blank description (stored as NULL).
func validateDescription(raw string) (*string, string) {
	d := strings.TrimSpace(raw)
	if d == "" {
		return nil, ""
	}
	if utf8.RuneCountInString(d) > maxDescriptionLen {
		return nil, fmt.Sprintf("must be at most %d characters", maxDescriptionLen)
	}
	return &d, ""
}

// validatePrice checks the value and normalises it to two fraction digits.
func validatePrice(n json.Number) (string, string) {
	s := strings.TrimSpace(n.String())
	if strings.HasPrefix(s, "-") {
		return "", "must be greater than or equal to 0"
	}
	if !pricePattern.MatchString(s) {
		return "", "must be a decimal number with at most 2 decimal places and at most 8 integer digits"
	}
	intPart, frac, _ := strings.Cut(s, ".")
	for len(frac) < 2 {
		frac += "0"
	}
	return intPart + "." + frac, ""
}

func validateStock(v int64) (int32, string) {
	switch {
	case v < 0:
		return 0, "must be greater than or equal to 0"
	case v > math.MaxInt32:
		return 0, fmt.Sprintf("must be at most %d", math.MaxInt32)
	}
	return int32(v), ""
}
