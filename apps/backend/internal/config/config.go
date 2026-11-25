package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds every runtime setting. Nothing outside this package reads os.Getenv
// directly, so a missing variable is a startup error instead of a silent zero value.
type Config struct {
	AppName   string
	Port      string
	PublicURL string

	DataDir string
	DBPath  string

	JWTSecret  []byte
	AccessTTL  time.Duration
	RefreshTTL time.Duration

	EncryptionKey []byte

	AdminEmail    string
	AdminPassword string

	ConnectorMode    string
	CloudflaredImage string
	DockerBin        string
	DockerNetwork    string
	// PanelInternalURL is what a connector container uses to reach this panel
	// when the dashboard itself is published through a tunnel.
	PanelInternalURL string

	CFAPIBaseURL string
}

// Load reads .env (if present), then resolves secrets.
func Load() (*Config, error) {
	// The panel keeps one .env at the repo root. `go run .` from apps/backend
	// therefore has to look up one level; a container passes plain env vars.
	for _, candidate := range []string{".env", "../../.env", "../.env"} {
		if _, err := os.Stat(candidate); err == nil {
			_ = godotenv.Load(candidate)
			break
		}
	}

	dataDir := env("DATA_DIR", "./data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	accessTTL, err := envInt("JWT_ACCESS_TOKEN_EXPIRATION_SECONDS", 1800)
	if err != nil {
		return nil, err
	}
	refreshDays, err := envInt("JWT_REFRESH_TOKEN_EXPIRATION_DAYS", 7)
	if err != nil {
		return nil, err
	}

	jwtSecret, err := resolveSecret("JWT_SECRET", filepath.Join(dataDir, ".jwt_secret"), func() (string, error) {
		return randomHex(32), nil
	})
	if err != nil {
		return nil, err
	}

	encKey, err := resolveSecret("ENCRYPTION_KEY", filepath.Join(dataDir, "encryption.key"), func() (string, error) {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString(raw), nil
	})
	if err != nil {
		return nil, err
	}
	keyBytes, err := decodeEncryptionKey(encKey)
	if err != nil {
		return nil, err
	}

	mode := strings.ToLower(env("CONNECTOR_MODE", "docker"))
	if mode != "docker" && mode != "external" {
		return nil, fmt.Errorf("CONNECTOR_MODE must be docker or external, got %q", mode)
	}

	return &Config{
		AppName:       env("APP_NAME", "CF Tunnel Panel"),
		Port:          env("PORT", "3001"),
		PublicURL:     strings.TrimRight(env("PUBLIC_URL", "http://localhost:7777"), "/"),
		DataDir:       dataDir,
		DBPath:        env("DB_PATH", filepath.Join(dataDir, "app.db")),
		JWTSecret:     []byte(jwtSecret),
		AccessTTL:     time.Duration(accessTTL) * time.Second,
		RefreshTTL:    time.Duration(refreshDays) * 24 * time.Hour,
		EncryptionKey: keyBytes,
		AdminEmail:    env("ADMIN_EMAIL", "admin@example.com"),
		// Simple default so a first start just works. seedAdmin logs a warning
		// when it is still in use, because the panel can be reachable from the
		// internet once it is published through a tunnel.
		AdminPassword:    env("ADMIN_PASSWORD", "admin123"),
		ConnectorMode:    mode,
		CloudflaredImage: env("CLOUDFLARED_IMAGE", "cloudflare/cloudflared:latest"),
		DockerBin:        env("DOCKER_BIN", "docker"),
		DockerNetwork:    os.Getenv("CONNECTOR_DOCKER_NETWORK"), // empty = docker default bridge
		PanelInternalURL: env("PANEL_INTERNAL_URL", "http://host.docker.internal:7777"),
		CFAPIBaseURL:     env("CF_API_BASE_URL", "https://api.cloudflare.com/client/v4"),
	}, nil
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer, got %q", key, raw)
	}
	return n, nil
}

// resolveSecret prefers the environment, falling back to a persisted file so a
// restarted container keeps the same key (otherwise every stored token is lost).
func resolveSecret(envKey, path string, generate func() (string, error)) (string, error) {
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
		return v, nil
	}
	if raw, err := os.ReadFile(path); err == nil {
		if v := strings.TrimSpace(string(raw)); v != "" {
			return v, nil
		}
	}
	secret, err := generate()
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(secret), 0o600); err != nil {
		return "", fmt.Errorf("persist secret to %s: %w", path, err)
	}
	return secret, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func decodeEncryptionKey(raw string) ([]byte, error) {
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	// Allow a raw 32+ byte passphrase as a convenience for local development.
	if len(raw) >= 32 {
		return []byte(raw[:32]), nil
	}
	return nil, fmt.Errorf("ENCRYPTION_KEY must be 32 bytes base64 encoded or at least 32 characters")
}
