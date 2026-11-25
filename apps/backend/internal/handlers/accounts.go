package handlers

import (
	"net/http"

	"cf-multi-tunnel/backend/internal/middleware"
	"cf-multi-tunnel/backend/internal/services"

	"github.com/labstack/echo/v4"
)

type AccountHandler struct{ accounts *services.AccountService }

func NewAccountHandler(accounts *services.AccountService) *AccountHandler {
	return &AccountHandler{accounts: accounts}
}

func (h *AccountHandler) List(c echo.Context) error {
	accounts, err := h.accounts.List(c.Request().Context(), middleware.Caller(c))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, accounts)
}

func (h *AccountHandler) Get(c echo.Context) error {
	account, err := h.accounts.Get(c.Request().Context(), middleware.Caller(c), c.Param("id"))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, account)
}

func (h *AccountHandler) Create(c echo.Context) error {
	var in services.CreateAccountInput
	if err := c.Bind(&in); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	account, err := h.accounts.CreateWithToken(c.Request().Context(), middleware.Caller(c), in)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusCreated, account)
}

func (h *AccountHandler) ImportTunnel(c echo.Context) error {
	var in services.ImportTunnelInput
	if err := c.Bind(&in); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	account, tunnel, err := h.accounts.ImportTunnelToken(c.Request().Context(), middleware.Caller(c), in)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusCreated, map[string]any{"account": account, "tunnel": tunnel})
}

func (h *AccountHandler) Rename(c echo.Context) error {
	var in struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&in); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	account, err := h.accounts.Rename(c.Request().Context(), middleware.Caller(c), c.Param("id"), in.Name)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, account)
}

func (h *AccountHandler) AttachToken(c echo.Context) error {
	var in struct {
		APIToken string `json:"api_token"`
	}
	if err := c.Bind(&in); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	account, err := h.accounts.AttachToken(c.Request().Context(), middleware.Caller(c), c.Param("id"), in.APIToken)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, account)
}

func (h *AccountHandler) Verify(c echo.Context) error {
	account, err := h.accounts.Verify(c.Request().Context(), middleware.Caller(c), c.Param("id"))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, account)
}

func (h *AccountHandler) Delete(c echo.Context) error {
	if err := h.accounts.Delete(c.Request().Context(), middleware.Caller(c), c.Param("id")); err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, map[string]bool{"ok": true})
}

func (h *AccountHandler) ListZones(c echo.Context) error {
	zones, err := h.accounts.ListZones(c.Request().Context(), middleware.Caller(c), c.Param("id"))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, zones)
}

func (h *AccountHandler) SyncZones(c echo.Context) error {
	zones, err := h.accounts.SyncZones(c.Request().Context(), middleware.Caller(c), c.Param("id"))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, zones)
}
