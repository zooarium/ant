package http

import (
	"context"
	"net/http"
	"time"

	"ant/internal/db"
	"ant/internal/platform/render"

	entsql "entgo.io/ent/dialect/sql"
)

// ReadyHandler pings the database with a short timeout and reports whether
// the service is ready to receive traffic. Distinct from /health, which is
// a static liveness check and never touches the DB.
func ReadyHandler(drv *entsql.Driver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := db.Ping(ctx, drv); err != nil {
			render.JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "NOT_READY", "error": err.Error()})
			return
		}
		render.JSON(w, http.StatusOK, map[string]string{"status": "READY"})
	}
}
