package handlers

import (
	"net/http"

	"cf-multi-tunnel/backend/internal/middleware"
	"cf-multi-tunnel/backend/internal/services"

	"github.com/labstack/echo/v4"
)

type DashboardHandler struct {
	dashboard *services.DashboardService
	connector *services.ConnectorService
}

func NewDashboardHandler(dashboard *services.DashboardService, connector *services.ConnectorService) *DashboardHandler {
	return &DashboardHandler{dashboard: dashboard, connector: connector}
}

func (h *DashboardHandler) Stats(c echo.Context) error {
	stats, err := h.dashboard.Stats(c.Request().Context(), middleware.Caller(c))
	if err != nil {
		return fail(c, err)
	}
	stats.ConnectorMode = ""
	status, err := h.connector.Status(c.Request().Context(), false)
	if err == nil {
		stats.ConnectorMode = status.Mode
		stats.DockerAvailable = status.DockerAvailable
	}
	return ok(c, http.StatusOK, stats)
}

type ConnectorHandler struct{ connector *services.ConnectorService }

func NewConnectorHandler(connector *services.ConnectorService) *ConnectorHandler {
	return &ConnectorHandler{connector: connector}
}

func (h *ConnectorHandler) Status(c echo.Context) error {
	checkRemote := c.QueryParam("check_remote") == "true"
	status, err := h.connector.Status(c.Request().Context(), checkRemote)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, status)
}

func (h *ConnectorHandler) Tags(c echo.Context) error {
	tags, err := h.connector.ListTags(c.Request().Context(), 30)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, tags)
}

func (h *ConnectorHandler) Update(c echo.Context) error {
	var in struct {
		Image string `json:"image"`
		Pin   bool   `json:"pin"`
	}
	if err := c.Bind(&in); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	result, err := h.connector.Update(c.Request().Context(), middleware.Caller(c), in.Image, in.Pin)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, result)
}

type AuditHandler struct{ audit *services.AuditService }

func NewAuditHandler(audit *services.AuditService) *AuditHandler { return &AuditHandler{audit: audit} }

func (h *AuditHandler) List(c echo.Context) error {
	logs, total, err := h.audit.List(c.Request().Context(), pagination(c))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, map[string]any{"items": logs, "total": total})
}
