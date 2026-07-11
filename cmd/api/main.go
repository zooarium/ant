package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"ant/docs"
	"ant/internal/attribute"
	"ant/internal/category"
	"ant/internal/db"
	"ant/internal/order"
	"ant/internal/ordergroup"
	platformhttp "ant/internal/platform/http"
	"ant/internal/product"
	"ant/internal/storefront"
	"ant/pkg/cache"
	"ant/pkg/captcha"
	"ant/pkg/config"
	"ant/pkg/keeper"

	"keeper/pkg/auth"

	"github.com/go-chi/chi/v5"
)

// @title Ant API
// @version 1.0
// @description This is the ant microservice.
// @host localhost:8082
// @BasePath /

// @tag.name Public
// @tag.description Endpoints under the /public prefix, reachable with keeper guest tokens via the order-intake listener.

// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

func main() {
	checkConfig := flag.Bool("check-config", false, "validate configuration (including secondary listeners) and exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		os.Exit(1)
	}

	if *checkConfig {
		enabled := 0
		for i := range cfg.Secondary {
			sec := &cfg.Secondary[i]
			if !sec.Enabled {
				continue
			}
			enabled++
			if err := platformhttp.ValidateRoutes(sec.Routes); err != nil {
				fmt.Printf("config invalid: %s: %v\n", sec.Name, err)
				os.Exit(1)
			}
		}
		fmt.Printf("config OK: primary %s, %d secondary listener(s) enabled\n", cfg.Server.Addr, enabled)
		os.Exit(0)
	}

	if err := os.MkdirAll(cfg.Log.Dir, 0755); err != nil {
		fmt.Printf("failed to create log directory: %v\n", err)
		os.Exit(1)
	}

	logFile, err := os.OpenFile(filepath.Join(cfg.Log.Dir, "api.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("failed to open log file: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := logFile.Close(); err != nil {
			fmt.Printf("failed to close log file: %v\n", err)
		}
	}()

	var logLevel slog.Level
	switch cfg.Log.Level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	mw := io.MultiWriter(os.Stdout, logFile)
	logger := slog.New(slog.NewJSONHandler(mw, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	docs.SwaggerInfo.Host = cfg.Server.Host

	client, err := db.NewClient(cfg.Database.Driver, cfg.Database.Path, cfg.Database.DSN)
	if err != nil {
		slog.Error("failed to open database client", "error", err, "driver", cfg.Database.Driver)
		os.Exit(1)
	}
	defer func() {
		if err := client.Close(); err != nil {
			slog.Error("failed to close database client", "error", err)
		}
	}()

	attributeRepo := attribute.NewRepository(client)
	attributeSvc := attribute.NewService(attributeRepo)
	attributeHandler := attribute.NewHandler(attributeSvc)

	categoryRepo := category.NewRepository(client)
	categorySvc := category.NewService(categoryRepo)
	categoryHandler := category.NewHandler(categorySvc)

	productRepo := product.NewRepository(client)
	productSvc := product.NewService(productRepo)
	productHandler := product.NewHandler(productSvc)

	// Keeper s2s client: supplies the tenant's app profile (tax rate) for
	// public order creation. Shared HTTP client with config timeout; profiles
	// cached in-memory for KEEPER.APP_TTL.
	keeperClient := keeper.NewClient(&http.Client{Timeout: cfg.Keeper.Timeout}, cfg.Keeper.BaseURL, cfg.Keeper.AppTTL)

	orderRepo := order.NewRepository(client)
	orderSvc := order.NewService(orderRepo, keeperClient)
	orderHandler := order.NewHandler(orderSvc, cfg.PublicOrder.MaxOrders, cfg.PublicOrder.Window)

	orderGroupRepo := ordergroup.NewRepository(client)
	orderGroupSvc := ordergroup.NewService(orderGroupRepo)
	orderGroupHandler := ordergroup.NewHandler(orderGroupSvc)

	storefrontRepo := storefront.NewRepository(client)
	storefrontSvc := storefront.NewService(storefrontRepo, cache.New(cfg.Cache.StorefrontTTL), productSvc, cfg.PublicStorefront.MaxProducts)
	storefrontHandler := storefront.NewHandler(storefrontSvc)

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.JWTExpiry)

	// Primary auth middleware. When impersonation is enabled it additionally
	// accepts keeper-minted impersonation tokens scoped to this service's
	// audience, enforcing audience match, read-only mode, and (optionally) live
	// revocation against keeper. Otherwise it is the plain JWT middleware.
	authMW := auth.Middleware(jwtManager)
	if cfg.Impersonation.Enabled {
		impMgr := auth.NewJWTManager(cfg.Impersonation.JWTSecret, 0)
		var revoked auth.RevocationChecker
		if cfg.Impersonation.RevocationCheck {
			revClient := &http.Client{Timeout: cfg.Impersonation.RevocationHTTP}
			revoked = auth.NewHTTPRevocationChecker(revClient, cfg.Impersonation.KeeperBaseURL, cfg.Impersonation.RevocationTTL)
		}
		authMW = auth.ImpersonationAwareMiddleware(jwtManager, impMgr, cfg.Impersonation.Audience, revoked)
		slog.Info("impersonation token acceptance enabled", "audience", cfg.Impersonation.Audience, "revocation_check", cfg.Impersonation.RevocationCheck)
	}

	// Captcha verifier for public write routes. Uses a shared HTTP client with
	// the timeout from config (never the zero-timeout default client). When
	// captcha is disabled this is a no-op verifier, so the wiring is identical.
	captchaClient := &http.Client{Timeout: cfg.Captcha.Timeout}
	captchaVerifier := captcha.New(cfg.Captcha.Enabled, cfg.Captcha.Secret, cfg.Captcha.MinScore, captchaClient)
	captchaMW := platformhttp.CaptchaMiddleware(captchaVerifier)

	mount := func(r chi.Router) {
		r.Mount("/attributes", attributeHandler.Routes())
		r.Mount("/categories", categoryHandler.Routes())
		r.Mount("/products", productHandler.Routes())
		r.Mount("/orders", orderHandler.Routes())
		r.Mount("/order-groups", orderGroupHandler.Routes())
		r.Mount("/storefront", storefrontHandler.Routes())
		// Public surface (reachable with guest tokens on the order-intake
		// listener). Write routes carry captcha verification internally; the
		// product catalog and tab reads are open.
		r.Route("/public", func(r chi.Router) {
			r.Mount("/categories", categoryHandler.PublicRoutes())
			r.Mount("/products", productHandler.PublicRoutes())
			r.Mount("/orders", orderHandler.PublicRoutes(captchaMW))
			r.Mount("/order-groups", orderGroupHandler.PublicRoutes(captchaMW))
			r.Mount("/storefront", storefrontHandler.PublicRoutes())
		})
	}

	router := platformhttp.NewRouter(cfg, authMW, mount)

	srv := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	go func() {
		slog.Info("starting server", "addr", srv.Addr, "env", cfg.Environment)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("failed to listen and serve", "error", err)
			os.Exit(1)
		}
	}()

	var secondarySrvs []*http.Server
	for i := range cfg.Secondary {
		sec := &cfg.Secondary[i]
		if !sec.Enabled {
			continue
		}

		secondaryRouter, err := platformhttp.NewSecondaryRouter(cfg, sec, jwtManager, mount)
		if err != nil {
			slog.Error("failed to build secondary router", "name", sec.Name, "error", err)
			os.Exit(1)
		}

		secondarySrv := &http.Server{
			Addr:         sec.Addr,
			Handler:      secondaryRouter,
			ReadTimeout:  cfg.Server.ReadTimeout,
			WriteTimeout: cfg.Server.WriteTimeout,
			IdleTimeout:  cfg.Server.IdleTimeout,
		}
		secondarySrvs = append(secondarySrvs, secondarySrv)

		go func() {
			slog.Info("starting secondary server", "name", sec.Name, "addr", secondarySrv.Addr, "routes", sec.Routes)
			if err := secondarySrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("failed to listen and serve on secondary", "name", sec.Name, "error", err)
				os.Exit(1)
			}
		}()
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	for _, secondarySrv := range secondarySrvs {
		if err := secondarySrv.Shutdown(ctx); err != nil {
			slog.Error("secondary server forced to shutdown", "addr", secondarySrv.Addr, "error", err)
			os.Exit(1)
		}
	}

	slog.Info("server exited gracefully")
}
