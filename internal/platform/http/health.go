package http

import (
	"bufio"
	"log/slog"
	"net/http"
	"os"
	"regexp"

	"ant/internal/platform/render"
)

// version is parsed once at startup from CHANGELOG.md (first "## [x.y.z]"
// heading); "dev" when the file is absent or has no released version yet.
var version = changelogVersion()

var versionRe = regexp.MustCompile(`^## \[(\d+\.\d+\.\d+)\]`)

func changelogVersion() string {
	f, err := os.Open("CHANGELOG.md")
	if err != nil {
		return "dev"
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		if m := versionRe.FindStringSubmatch(s.Text()); m != nil {
			return m[1]
		}
	}
	return "dev"
}

// HealthHandler returns a simple 200 OK status with the service version.
// @Summary Check service health
// @Description Get the health status and version of the service
// @Tags health
// @Produce json
// @Success 200 {object} render.Response
// @Router /health [get]
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	slog.Info("health check requested", "remote_addr", r.RemoteAddr)
	render.JSON(w, http.StatusOK, map[string]string{"status": "UP", "version": version})
}
