package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"cf-multi-tunnel/backend/internal/middleware"
	"cf-multi-tunnel/backend/internal/services"

	"github.com/labstack/echo/v4"
)

type TunnelHandler struct {
	tunnels *services.TunnelService
	apps    *services.AppService
}

func NewTunnelHandler(tunnels *services.TunnelService, apps *services.AppService) *TunnelHandler {
	return &TunnelHandler{tunnels: tunnels, apps: apps}
}

func (h *TunnelHandler) List(c echo.Context) error {
	tunnels, err := h.tunnels.List(c.Request().Context(), middleware.Caller(c), c.QueryParam("account_id"), c.QueryParam("location"))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, tunnels)
}

func (h *TunnelHandler) Get(c echo.Context) error {
	tunnel, err := h.tunnels.Get(c.Request().Context(), middleware.Caller(c), c.Param("id"))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, tunnel)
}

func (h *TunnelHandler) Create(c echo.Context) error {
	var in services.CreateTunnelInput
	if err := c.Bind(&in); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	tunnel, err := h.tunnels.Create(c.Request().Context(), middleware.Caller(c), in)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusCreated, tunnel)
}

func (h *TunnelHandler) Sync(c echo.Context) error {
	var in struct {
		AccountID string `json:"account_id"`
	}
	if err := c.Bind(&in); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	if strings.TrimSpace(in.AccountID) == "" {
		return fail(c, services.Validation(map[string]string{"account_id": "required"}))
	}
	result, err := h.tunnels.ReconcileAccount(c.Request().Context(), middleware.Caller(c), in.AccountID)
	if err != nil {
		return fail(c, err)
	}
	tunnels, err := h.tunnels.List(c.Request().Context(), middleware.Caller(c), in.AccountID, "")
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, map[string]any{"result": result, "tunnels": tunnels})
}

func (h *TunnelHandler) Delete(c echo.Context) error {
	if err := h.tunnels.Delete(c.Request().Context(), middleware.Caller(c), c.Param("id")); err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, map[string]bool{"ok": true})
}

// Update changes where the tunnel lives (local vs remote) and how it runs.
func (h *TunnelHandler) Update(c echo.Context) error {
	var in services.UpdateTunnelInput
	if err := c.Bind(&in); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	tunnel, err := h.tunnels.Update(c.Request().Context(), middleware.Caller(c), c.Param("id"), in)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, tunnel)
}

func (h *TunnelHandler) Start(c echo.Context) error {
	tunnel, err := h.tunnels.Start(c.Request().Context(), middleware.Caller(c), c.Param("id"))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, tunnel)
}

func (h *TunnelHandler) Stop(c echo.Context) error {
	tunnel, err := h.tunnels.Stop(c.Request().Context(), middleware.Caller(c), c.Param("id"))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, tunnel)
}

func (h *TunnelHandler) Logs(c echo.Context) error {
	lines, _ := strconv.Atoi(c.QueryParam("lines"))
	logs, err := h.tunnels.Logs(c.Request().Context(), middleware.Caller(c), c.Param("id"), lines)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, map[string]string{"logs": logs})
}

func (h *TunnelHandler) ConfigYAML(c echo.Context) error {
	cfg, err := h.tunnels.ConfigYAML(c.Request().Context(), middleware.Caller(c), c.Param("id"))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, map[string]string{"config": cfg})
}

func (h *TunnelHandler) CredentialsJSON(c echo.Context) error {
	creds, err := h.tunnels.CredentialsJSON(c.Request().Context(), middleware.Caller(c), c.Param("id"))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, map[string]string{"credentials": creds})
}

func (h *TunnelHandler) Token(c echo.Context) error {
	tunnel, err := h.tunnels.Get(c.Request().Context(), middleware.Caller(c), c.Param("id"))
	if err != nil {
		return fail(c, err)
	}
	token, err := h.tunnels.TokenFor(c.Request().Context(), tunnel)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, map[string]string{"token": token})
}

// Apps nested under a tunnel.
func (h *TunnelHandler) ListApps(c echo.Context) error {
	apps, err := h.apps.ListByTunnel(c.Request().Context(), middleware.Caller(c), c.Param("id"))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, apps)
}

func (h *TunnelHandler) CreateApp(c echo.Context) error {
	var in services.CreateAppInput
	if err := c.Bind(&in); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	app, err := h.apps.Create(c.Request().Context(), middleware.Caller(c), c.Param("id"), in)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusCreated, app)
}

type AppHandler struct{ apps *services.AppService }

func NewAppHandler(apps *services.AppService) *AppHandler { return &AppHandler{apps: apps} }

func (h *AppHandler) List(c echo.Context) error {
	apps, err := h.apps.List(c.Request().Context(), middleware.Caller(c))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, apps)
}

func (h *AppHandler) Get(c echo.Context) error {
	app, err := h.apps.Get(c.Request().Context(), middleware.Caller(c), c.Param("id"))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, app)
}

func (h *AppHandler) Update(c echo.Context) error {
	var in services.CreateAppInput
	if err := c.Bind(&in); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	app, err := h.apps.Update(c.Request().Context(), middleware.Caller(c), c.Param("id"), in)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, app)
}

func (h *AppHandler) Delete(c echo.Context) error {
	if err := h.apps.Delete(c.Request().Context(), middleware.Caller(c), c.Param("id")); err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, map[string]bool{"ok": true})
}

func (h *AppHandler) SetEnabled(c echo.Context) error {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.Bind(&in); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	app, err := h.apps.SetEnabled(c.Request().Context(), middleware.Caller(c), c.Param("id"), in.Enabled)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, app)
}

func (h *AppHandler) Retry(c echo.Context) error {
	app, err := h.apps.Retry(c.Request().Context(), middleware.Caller(c), c.Param("id"))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, app)
}

func (h *AppHandler) GetPanelAddress(c echo.Context) error {
	return ok(c, http.StatusOK, h.apps.PanelAddress())
}

func (h *AppHandler) GetPanelExposure(c echo.Context) error {
	app, err := h.apps.GetPanelExposure(c.Request().Context())
	if err != nil {
		return fail(c, err)
	}
	if app == nil {
		return ok(c, http.StatusOK, nil)
	}
	return ok(c, http.StatusOK, app)
}

func (h *AppHandler) SetPanelExposure(c echo.Context) error {
	var in services.PanelExposureInput
	if err := c.Bind(&in); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	app, err := h.apps.SetPanelExposure(c.Request().Context(), middleware.Caller(c), in)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, app)
}

func (h *AppHandler) RemovePanelExposure(c echo.Context) error {
	if err := h.apps.RemovePanelExposure(c.Request().Context(), middleware.Caller(c)); err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, map[string]bool{"ok": true})
}
