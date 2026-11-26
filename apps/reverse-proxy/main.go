// Command reverse-proxy serves the built SPA and proxies /api to the backend on
// a single origin. This is what makes the panel reachable at one hostname and
// what keeps CORS out of the picture in production.
package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	for _, candidate := range []string{".env", "../../.env", "../.env"} {
		if _, err := os.Stat(candidate); err == nil {
			_ = godotenv.Load(candidate)
			break
		}
	}

	// PROXY_PORT wins over PORT so a native run can share one .env with the
	// backend (PORT=3001) without the two processes fighting over the same port.
	port := getEnv("PROXY_PORT", getEnv("PORT", "7777"))
	backendURL := getEnv("BACKEND_URL", "http://localhost:3001")
	staticDir := getEnv("STATIC_DIR", "../frontend/dist")

	target, err := url.Parse(backendURL)
	if err != nil {
		log.Fatalf("invalid BACKEND_URL %q: %v", backendURL, err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("backend proxy error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"code":"backend_unavailable","message":"backend is not reachable"}`))
	}

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.Recover())
	e.Use(middleware.Gzip())

	// Long-lived assets are immutable because Vite hashes their names.
	e.GET("/assets/*", echo.WrapHandler(http.StripPrefix("/", http.FileServer(http.Dir(staticDir)))))

	e.Any("/api/*", echo.WrapHandler(proxy))
	e.Any("/healthz", echo.WrapHandler(proxy))

	// SPA fallback: any other path serves index.html so client-side routing works.
	index := filepath.Join(staticDir, "index.html")
	e.GET("/*", func(c echo.Context) error {
		path := c.Request().URL.Path
		if fileExists(filepath.Join(staticDir, strings.TrimPrefix(path, "/"))) {
			return c.File(filepath.Join(staticDir, strings.TrimPrefix(path, "/")))
		}
		if _, err := os.Stat(index); err != nil {
			return c.String(http.StatusNotFound, "frontend build not found; run `bun run build:frontend`")
		}
		c.Response().Header().Set("Cache-Control", "no-cache")
		return c.File(index)
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           e,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("reverse proxy listening on :%s -> backend %s, static %s", port, backendURL, staticDir)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
