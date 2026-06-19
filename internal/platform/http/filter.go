package http

import (
	"net/http"
	"strconv"
)

// ParseStatusFilter extracts an optional status query parameter. A missing or
// non-integer value yields nil (no filter), mirroring ParsePagination's
// fallback behaviour.
func ParseStatusFilter(r *http.Request) *int8 {
	v := r.URL.Query().Get("status")
	if v == "" {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 8)
	if err != nil {
		return nil
	}
	s := int8(n)
	return &s
}

// ParseCategoryFilter extracts an optional category_id query parameter. A
// missing or non-integer value yields nil (no filter). When set, the product
// list is scoped to that category and its entire subtree.
func ParseCategoryFilter(r *http.Request) *int {
	v := r.URL.Query().Get("category_id")
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil
	}
	return &n
}
