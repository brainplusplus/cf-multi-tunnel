package handlers

import (
	"net/http"

	"cf-multi-tunnel/backend/internal/middleware"
	"cf-multi-tunnel/backend/internal/services"

	"github.com/labstack/echo/v4"
)

// DNSHandler is a thin shell around DNSService: validation, ownership and the
// managed-record lock all live in the service so a future CLI/MCP reuses them.
type DNSHandler struct{ dns *services.DNSService }

func NewDNSHandler(dns *services.DNSService) *DNSHandler { return &DNSHandler{dns: dns} }

func (h *DNSHandler) Zones(c echo.Context) error {
	zones, err := h.dns.ListZones(c.Request().Context(), middleware.Caller(c))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, map[string]any{"items": zones})
}

func (h *DNSHandler) SyncZones(c echo.Context) error {
	result, err := h.dns.SyncZones(c.Request().Context(), middleware.Caller(c))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, result)
}

func (h *DNSHandler) Records(c echo.Context) error {
	items, truncated, err := h.dns.ListRecords(c.Request().Context(), middleware.Caller(c), c.Param("zoneID"))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, map[string]any{"items": items, "truncated": truncated})
}

func (h *DNSHandler) CreateRecord(c echo.Context) error {
	var in services.DNSRecordInput
	if err := c.Bind(&in); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	record, err := h.dns.CreateRecord(c.Request().Context(), middleware.Caller(c), c.Param("zoneID"), in)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusCreated, record)
}

func (h *DNSHandler) UpdateRecord(c echo.Context) error {
	var in services.DNSRecordInput
	if err := c.Bind(&in); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	record, err := h.dns.UpdateRecord(c.Request().Context(), middleware.Caller(c), c.Param("zoneID"), c.Param("recordID"), in)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, record)
}

func (h *DNSHandler) DeleteRecord(c echo.Context) error {
	err := h.dns.DeleteRecord(c.Request().Context(), middleware.Caller(c), c.Param("zoneID"), c.Param("recordID"))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, map[string]bool{"ok": true})
}
