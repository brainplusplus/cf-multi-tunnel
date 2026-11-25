package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"cf-multi-tunnel/backend/internal/config"
	"cf-multi-tunnel/backend/internal/infrastructure"
	"cf-multi-tunnel/backend/internal/models"
	"cf-multi-tunnel/backend/internal/pkg/cryptox"
	"cf-multi-tunnel/backend/internal/pkg/logger"
	db "cf-multi-tunnel/backend/internal/repositories/db"
	"cf-multi-tunnel/backend/internal/routes"
	"cf-multi-tunnel/backend/internal/services"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gorm.io/gorm"
)

func main() {
	logger.Init()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("configuration error", "error", err)
	}

	dbConn, err := infrastructure.InitDB(cfg.DBPath)
	if err != nil {
		logger.Fatal("database error", "error", err)
	}
	defer infrastructure.CloseDB(dbConn)

	enc, err := cryptox.New(cfg.EncryptionKey)
	if err != nil {
		logger.Fatal("encryption key error", "error", err)
	}

	if err := seedAdmin(cfg, dbConn); err != nil {
		logger.Fatal("failed to seed admin", "error", err)
	}

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = httpErrorHandler
	e.Use(middleware.Recover())
	e.Use(middleware.BodyLimit("2M"))
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogMethod:  true,
		LogURI:     true,
		LogStatus:  true,
		LogLatency: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			slog.Info("request", "method", v.Method, "uri", v.URI, "status", v.Status, "latency", v.Latency.String())
			return nil
		},
	}))
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		// The SPA is served by the reverse proxy on the same origin in
		// production; CORS only matters for the Vite dev server.
		AllowOrigins: []string{"http://localhost:3002", "http://127.0.0.1:3002", "http://localhost:7777"},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAuthorization},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
	}))

	e.GET("/healthz", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	svc := routes.Setup(e, dbConn, cfg, enc)

	// Reconcile desired state on boot: tunnels marked running are restarted,
	// and ingress is re-pushed so a container that died mid-write recovers.
	go reconcileOnBoot(svc, cfg)

	// Periodic status refresh. Cloudflare connections and Docker state are both
	// polled here so the UI never shows stale "running" for a dead connector.
	go statusWorker(svc, cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("panel listening", "port", cfg.Port, "connector_mode", cfg.ConnectorMode)
		if err := e.Start(":" + cfg.Port); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("server error", "error", err)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}

func httpErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}
	status := http.StatusInternalServerError
	message := "internal server error"

	var httpErr *echo.HTTPError
	if errors.As(err, &httpErr) {
		status = httpErr.Code
		if msg, ok := httpErr.Message.(string); ok {
			message = msg
		}
	}
	if status >= 500 {
		slog.Error("unhandled request error", "error", err)
	}
	_ = c.JSON(status, map[string]string{"code": "error", "message": message})
}

// reconcileOnBoot restarts tunnels that were supposed to be running and pushes
// ingress for remote-configurable ones, so a panel restart is not an outage.
func reconcileOnBoot(svc *routes.Services, cfg *config.Config) {
	if cfg.ConnectorMode != "docker" {
		return
	}
	time.Sleep(2 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	tunnels, err := svc.Tunnels.List(ctx, services.Caller{Admin: true}, "", "")
	if err != nil {
		slog.Error("boot reconcile: could not list tunnels", "error", err)
		return
	}
	for i := range tunnels {
		t := &tunnels[i]
		if !t.ManagedLocally() || !t.DesiredRunning {
			continue
		}
		if err := svc.Connector.EnsureRunning(ctx, t); err != nil {
			slog.Error("boot reconcile: start failed", "tunnel", t.Name, "error", err)
			continue
		}
		svc.Connector.RecordRunningVersion(ctx, t)
		slog.Info("boot reconcile: tunnel started", "tunnel", t.Name)
	}
}

func statusWorker(svc *routes.Services, cfg *config.Config) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		svc.Tunnels.RefreshAll(ctx)
		cancel()
	}
}

func seedAdmin(cfg *config.Config, dbConn *gorm.DB) error {
	repo := db.NewUserRepository(dbConn)
	count, err := repo.Count(context.Background())
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	// Creating the very first admin with a well-known password would hand the
	// panel to whoever finds the URL first. Refuse instead of guessing: the
	// operator must choose a password.
	// Only an empty password is refused: it would create an unusable admin.
	// Anything else is accepted so first start stays simple, but a weak one is
	// announced loudly because the panel can be published on the internet.
	pw := strings.TrimSpace(cfg.AdminPassword)
	if pw == "" {
		return fmt.Errorf("ADMIN_PASSWORD is empty; set any password in .env before first start")
	}
	if len(pw) < 10 || pw == "admin123" {
		slog.Warn("first admin created with a weak password",
			"email", cfg.AdminEmail,
			"hint", "change it in the UI under Account → Change password if this panel is reachable from the internet")
	}

	hashed, err := services.HashPassword(cfg.AdminPassword)
	if err != nil {
		return err
	}
	admin := &models.User{
		Email:    cfg.AdminEmail,
		Password: hashed,
		Name:     "Administrator",
		Role:     models.RoleAdmin,
		IsActive: true,
	}
	if err := repo.Create(context.Background(), admin); err != nil {
		return err
	}
	slog.Info("created initial admin", "email", cfg.AdminEmail)
	return nil
}
