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
