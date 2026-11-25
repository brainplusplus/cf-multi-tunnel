package middleware

import (
	"net/http"
	"strings"

	"cf-multi-tunnel/backend/internal/services"

	"github.com/labstack/echo/v4"
)

const callerKey = "caller"

// Auth validates the bearer token and stores the caller on the request context.
func Auth(auth *services.AuthService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				return c.JSON(http.StatusUnauthorized, map[string]string{"code": "unauthorized", "message": "missing bearer token"})
			}

			claims, err := auth.ValidateAccess(parts[1])
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"code": "unauthorized", "message": "invalid or expired access token"})
			}

			// Re-read the user so a deactivated or demoted account loses access
			// immediately instead of waiting for the token to expire.
			user, err := auth.GetUser(c.Request().Context(), claims.UserID)
			if err != nil || !user.IsActive {
				return c.JSON(http.StatusUnauthorized, map[string]string{"code": "unauthorized", "message": "account is not active"})
			}

			c.Set(callerKey, services.Caller{ID: user.ID, Email: user.Email, Admin: user.IsAdmin()})
			return next(c)
		}
	}
}

func Caller(c echo.Context) services.Caller {
	if v, ok := c.Get(callerKey).(services.Caller); ok {
		return v
	}
	return services.Caller{}
}

func AdminOnly(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !Caller(c).Admin {
			return c.JSON(http.StatusForbidden, map[string]string{"code": "forbidden", "message": "admin role required"})
		}
		return next(c)
	}
}
