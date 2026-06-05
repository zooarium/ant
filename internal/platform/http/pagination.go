package http

import (
	"net/http"
	"strconv"
)

// Pagination holds normalized list query parameters.
type Pagination struct {
	Limit  int
	Offset int
}

const (
	defaultLimit = 50
	maxLimit     = 500
	minLimit     = 1
)

// ParsePagination extracts limit/offset from the request query string and
// clamps them to safe bounds. Non-integer values fall back to defaults rather
// than producing an error.
//
//	limit:  default 50, min 1, max 500
//	offset: default 0, min 0
func ParsePagination(r *http.Request) Pagination {
	limit := defaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if limit < minLimit {
		limit = minLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}
	if offset < 0 {
		offset = 0
	}

	return Pagination{Limit: limit, Offset: offset}
}
