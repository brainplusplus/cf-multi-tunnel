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
	pub := user.Public()
	return ok(c, http.StatusOK, map[string]any{
		"id": pub.ID, "email": pub.Email, "name": pub.Name, "role": pub.Role,
		"is_active": pub.IsActive, "created_at": pub.CreatedAt,
		// Lets the SPA render the "viewing as" banner after a reload, when the
		// impersonation token is the only thing in localStorage.
		"impersonated_by": caller.ImpersonatedBy,
	})
}

type impersonateRequest struct {
	UserID string `json:"user_id"`
}

// Impersonate handles POST /api/auth/impersonate. Admin only: the service
// re-checks the role, so this route cannot be widened by a middleware mistake.
func (h *AuthHandler) Impersonate(c echo.Context) error {
	var req impersonateRequest
	if err := c.Bind(&req); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	result, err := h.auth.Impersonate(c.Request().Context(), middleware.Caller(c), req.UserID, clientIP(c))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, result)
}

// ExitImpersonation only writes the audit trail. The browser restores the
// admin's own token, so there is no server-side session to end.
func (h *AuthHandler) ExitImpersonation(c echo.Context) error {
	caller := middleware.Caller(c)
	h.auth.EndImpersonation(c.Request().Context(), caller, caller.ImpersonatedBy, clientIP(c))
	return ok(c, http.StatusOK, map[string]bool{"ok": true})
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
