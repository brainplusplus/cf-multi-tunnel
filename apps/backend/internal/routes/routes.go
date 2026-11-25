package routes

import (
	"cf-multi-tunnel/backend/internal/config"
	"cf-multi-tunnel/backend/internal/connector"
	"cf-multi-tunnel/backend/internal/handlers"
	"cf-multi-tunnel/backend/internal/middleware"
	"cf-multi-tunnel/backend/internal/pkg/cryptox"
	db "cf-multi-tunnel/backend/internal/repositories/db"
	"cf-multi-tunnel/backend/internal/services"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// Services is the assembled dependency graph, returned so main can start the
// background worker and the connector with the same instances.
type Services struct {
	Auth       *services.AuthService
	Tunnels    *services.TunnelService
	Connector  *services.ConnectorService
	Supervisor *connector.Supervisor
}

func Setup(e *echo.Echo, dbConn *gorm.DB, cfg *config.Config, enc *cryptox.Cipher) *Services {
	// ── Repositories ────────────────────────────────────────────────
	userRepo := db.NewUserRepository(dbConn)
	tokenRepo := db.NewRefreshTokenRepository(dbConn)
	accountRepo := db.NewAccountRepository(dbConn)
	zoneRepo := db.NewZoneRepository(dbConn)
	tunnelRepo := db.NewTunnelRepository(dbConn)
	appRepo := db.NewAppRepository(dbConn)
	auditRepo := db.NewAuditRepository(dbConn)
	settingRepo := db.NewSettingRepository(dbConn)

	// ── Connector ───────────────────────────────────────────────────
	sup := connector.New(cfg)
	auditSvc := services.NewAuditService(auditRepo)
	connectorSvc := services.NewConnectorService(sup, settingRepo, tunnelRepo, cfg, auditSvc)
	sup.SetImageProvider(connectorSvc.Image)

	// ── Services ────────────────────────────────────────────────────
	authSvc := services.NewAuthService(userRepo, tokenRepo, cfg, auditSvc)
	userSvc := services.NewUserService(userRepo, auditSvc)
	cf := services.NewCF(cfg, enc)
	accountSvc := services.NewAccountService(accountRepo, zoneRepo, tunnelRepo, appRepo, cf, auditSvc, connectorSvc)
	tunnelSvc := services.NewTunnelService(tunnelRepo, accountRepo, zoneRepo, appRepo, cf, auditSvc, connectorSvc, cfg)
	appSvc := services.NewAppService(appRepo, tunnelRepo, accountRepo, zoneRepo, tunnelSvc, cf, auditSvc, cfg)
	// Adding or upgrading a token must pull in the tunnels and hostnames that
	// already exist on Cloudflare. Wired here because the tunnel service is
	// built after the account service.
	accountSvc.SetTunnelReconciler(tunnelSvc)
	dashboardSvc := services.NewDashboardService(accountRepo, tunnelRepo, appRepo, userRepo)
	sup.SetTokenProvider(tunnelSvc)

	// ── Handlers ────────────────────────────────────────────────────
	authH := handlers.NewAuthHandler(authSvc)
	userH := handlers.NewUserHandler(userSvc)
	accountH := handlers.NewAccountHandler(accountSvc)
	tunnelH := handlers.NewTunnelHandler(tunnelSvc, appSvc)
	appH := handlers.NewAppHandler(appSvc)
	dashboardH := handlers.NewDashboardHandler(dashboardSvc, connectorSvc)
	connectorH := handlers.NewConnectorHandler(connectorSvc)
	auditH := handlers.NewAuditHandler(auditSvc)

	// ── Routes ──────────────────────────────────────────────────────
	api := e.Group("/api")

	auth := api.Group("/auth")
	auth.POST("/login", authH.Login)
	auth.POST("/refresh", authH.Refresh)
	auth.POST("/logout", authH.Logout)
	auth.GET("/me", authH.Me, middleware.Auth(authSvc))
	auth.POST("/change-password", authH.ChangePassword, middleware.Auth(authSvc))

	protected := api.Group("", middleware.Auth(authSvc))

	protected.GET("/dashboard/stats", dashboardH.Stats)

	protected.GET("/cf-accounts", accountH.List)
	protected.GET("/cf-accounts/:id", accountH.Get)
	protected.POST("/cf-accounts", accountH.Create)
	protected.POST("/cf-accounts/probe", accountH.ProbeToken)
	protected.POST("/cf-accounts/import-tunnel", accountH.ImportTunnel)
	protected.PATCH("/cf-accounts/:id", accountH.Rename)
	protected.POST("/cf-accounts/:id/attach-token", accountH.AttachToken)
	protected.POST("/cf-accounts/:id/verify", accountH.Verify)
	protected.DELETE("/cf-accounts/:id", accountH.Delete)
	protected.GET("/cf-accounts/:id/zones", accountH.ListZones)
	protected.POST("/cf-accounts/:id/sync-zones", accountH.SyncZones)

	protected.GET("/tunnels", tunnelH.List)
	protected.GET("/tunnels/:id", tunnelH.Get)
	protected.POST("/tunnels", tunnelH.Create)
	protected.POST("/tunnels/sync", tunnelH.Sync)
	protected.PATCH("/tunnels/:id", tunnelH.Update)
	protected.DELETE("/tunnels/:id", tunnelH.Delete)
	protected.POST("/tunnels/:id/start", tunnelH.Start)
	protected.POST("/tunnels/:id/stop", tunnelH.Stop)
	protected.GET("/tunnels/:id/logs", tunnelH.Logs)
	protected.GET("/tunnels/:id/token", tunnelH.Token)
	protected.GET("/tunnels/:id/config.yml", tunnelH.ConfigYAML)
	protected.GET("/tunnels/:id/credentials.json", tunnelH.CredentialsJSON)
	protected.GET("/tunnels/:id/apps", tunnelH.ListApps)
	protected.POST("/tunnels/:id/apps", tunnelH.CreateApp)

	protected.GET("/apps", appH.List)
	protected.GET("/apps/:id", appH.Get)
	protected.PUT("/apps/:id", appH.Update)
	protected.DELETE("/apps/:id", appH.Delete)
	protected.PATCH("/apps/:id/enabled", appH.SetEnabled)
	protected.POST("/apps/:id/retry", appH.Retry)

	// Panel exposure is admin-only: it republishes the control plane itself.
	protected.GET("/settings/panel-address", appH.GetPanelAddress, middleware.AdminOnly)
	protected.GET("/settings/panel-exposure", appH.GetPanelExposure, middleware.AdminOnly)
	protected.PUT("/settings/panel-exposure", appH.SetPanelExposure, middleware.AdminOnly)
	protected.DELETE("/settings/panel-exposure", appH.RemovePanelExposure, middleware.AdminOnly)

	admin := protected.Group("/admin", middleware.AdminOnly)
	admin.GET("/users", userH.List)
	admin.GET("/users/:id", userH.Get)
	admin.POST("/users", userH.Create)
	admin.PUT("/users/:id", userH.Update)
	admin.DELETE("/users/:id", userH.Delete)
	admin.GET("/audit-logs", auditH.List)
	admin.GET("/connector", connectorH.Status)
	admin.GET("/connector/tags", connectorH.Tags)
	admin.POST("/connector/update", connectorH.Update)

	return &Services{Auth: authSvc, Tunnels: tunnelSvc, Connector: connectorSvc, Supervisor: sup}
}
