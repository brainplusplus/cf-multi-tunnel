package handlers

import (
	"net/http"

	"cf-multi-tunnel/backend/internal/middleware"
	"cf-multi-tunnel/backend/internal/services"

	"github.com/labstack/echo/v4"
)

type UserHandler struct{ users *services.UserService }

func NewUserHandler(users *services.UserService) *UserHandler { return &UserHandler{users: users} }

func (h *UserHandler) List(c echo.Context) error {
	users, total, err := h.users.List(c.Request().Context(), c.QueryParam("search"), pagination(c))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, map[string]any{
		"items": users,
		"total": total,
		"page":  c.QueryParam("page"),
	})
}

func (h *UserHandler) Get(c echo.Context) error {
	user, err := h.users.Get(c.Request().Context(), c.Param("id"))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, user)
}

func (h *UserHandler) Create(c echo.Context) error {
	var in services.CreateUserInput
	if err := c.Bind(&in); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	user, err := h.users.Create(c.Request().Context(), middleware.Caller(c), in)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusCreated, user)
}

func (h *UserHandler) Update(c echo.Context) error {
	var in services.UpdateUserInput
	if err := c.Bind(&in); err != nil {
		return fail(c, services.BadRequest("invalid request body"))
	}
	user, err := h.users.Update(c.Request().Context(), middleware.Caller(c), c.Param("id"), in)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, user)
}

func (h *UserHandler) Delete(c echo.Context) error {
	if err := h.users.Delete(c.Request().Context(), middleware.Caller(c), c.Param("id")); err != nil {
		return fail(c, err)
	}
	return ok(c, http.StatusOK, map[string]bool{"ok": true})
}
