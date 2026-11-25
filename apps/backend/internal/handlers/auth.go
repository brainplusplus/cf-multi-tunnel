package handlers

import (
	"net/http"

	"cf-multi-tunnel/backend/internal/middleware"
	"cf-multi-tunnel/backend/internal/services"

	"github.com/labstack/echo/v4"
)

type AuthHandler struct{ auth *services.AuthService }

func NewAuthHandler(auth *services.AuthService) *AuthHandler { return &AuthHandler{auth: auth} }

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	result, err := h.auth.Login(c.Request().Context(), req.Email, req.Password, c.Request().UserAgent(), clientIP(c))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, result)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *AuthHandler) Refresh(c echo.Context) error {
	var req refreshRequest
	if err := c.Bind(&req); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	result, err := h.auth.Refresh(c.Request().Context(), req.RefreshToken, c.Request().UserAgent(), clientIP(c))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, result)
}

func (h *AuthHandler) Logout(c echo.Context) error {
	var req refreshRequest
	_ = c.Bind(&req)
	h.auth.Logout(c.Request().Context(), req.RefreshToken)
	return ok(c, http.StatusOK, map[string]bool{"ok": true})
}

func (h *AuthHandler) Me(c echo.Context) error {
	caller := middleware.Caller(c)
	user, err := h.auth.GetUser(c.Request().Context(), caller.ID)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, user.Public())
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h *AuthHandler) ChangePassword(c echo.Context) error {
	var req changePasswordRequest
	if err := c.Bind(&req); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	caller := middleware.Caller(c)
	if err := h.auth.ChangePassword(c.Request().Context(), caller.ID, req.CurrentPassword, req.NewPassword); err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, map[string]bool{"ok": true})
}
