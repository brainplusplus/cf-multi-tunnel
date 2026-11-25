package handlers

import (
	"net/http"
	"strconv"

	db "cf-multi-tunnel/backend/internal/repositories/db"
	"cf-multi-tunnel/backend/internal/services"

	"github.com/labstack/echo/v4"
)

// fail is the single place an error becomes an HTTP response. Anything that is
// not an *AppError is logged and reported as a generic 500.
func fail(c echo.Context, err error) error {
	appErr := services.AsAppError(err)
	if appErr.Status >= http.StatusInternalServerError {
		c.Logger().Error(err)
	}
	return c.JSON(appErr.Status, appErr)
}

func ok(c echo.Context, status int, data any) error {
	return c.JSON(status, data)
}

func pagination(c echo.Context) db.Pagination {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	size, _ := strconv.Atoi(c.QueryParam("page_size"))
	return db.Pagination{Page: page, PageSize: size}
}

// clientIP prefers Cloudflare's header when the panel sits behind its own
// tunnel, otherwise falls back to the socket address.
func clientIP(c echo.Context) string {
	if ip := c.Request().Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	return c.RealIP()
}
