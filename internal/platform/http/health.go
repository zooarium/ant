package http

import (
	"log/slog"
	"net/http"

	"ant/internal/platform/render"
)

// HealthHandler returns a simple 200 OK status.
// @Summary Check service health
// @Description Get the health status of the service
// @Tags health
// @Produce json
// @Success 200 {object} render.Response
// @Router /health [get]
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	slog.Info("health check requested", "remote_addr", r.RemoteAddr)
	render.JSON(w, http.StatusOK, map[string]string{"status": "UP"})
}
