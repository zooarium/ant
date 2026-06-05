package http

import (
	"time"

	_ "ant/docs"
	"ant/pkg/config"

	"keeper/pkg/auth"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// NewRouter creates a new chi router with middleware and routes.
// Add entity handlers as parameters and mount them in the protected group below.
func NewRouter(cfg *config.Config, jwtManager *auth.JWTManager) *chi.Mux {
	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: cfg.CORS.AllowedOrigins,
		AllowedMethods: []string{"GET", "POST", "OPTIONS", "PUT", "DELETE"},
		AllowedHeaders: []string{"Origin", "Content-Type", "Authorization"},
	}))
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(MetricsMiddleware)
	r.Use(httprate.LimitByIP(100, 1*time.Minute))

	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	r.Get("/health", HealthHandler)

	// Prometheus metrics endpoint, exempt from JWT auth.
	r.Handle("/metrics", promhttp.Handler())

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(jwtManager))
		// Mount entity handlers here, e.g.:
		// r.Mount("/things", thingHandler.Routes())
	})

	return r
}
