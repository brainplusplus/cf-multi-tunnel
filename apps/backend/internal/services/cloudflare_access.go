package services

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"cf-multi-tunnel/backend/internal/cloudflare"
	"cf-multi-tunnel/backend/internal/config"
	"cf-multi-tunnel/backend/internal/models"
	"cf-multi-tunnel/backend/internal/pkg/cryptox"
	db "cf-multi-tunnel/backend/internal/repositories/db"
)

// CF is the shared factory for Cloudflare clients. Every call decrypts the
// account token just-in-time and never keeps it in memory longer than needed.
type CF struct {
	cfg *config.Config
	enc *cryptox.Cipher
}

func NewCF(cfg *config.Config, enc *cryptox.Cipher) *CF {
	return &CF{cfg: cfg, enc: enc}
}

func (c *CF) ClientFor(account *models.CFAccount) (*cloudflare.Client, error) {
	if !account.HasAPI() {
		return nil, BadRequest("this account was added with a tunnel token only; add a scoped API token to manage it")
	}
	token, err := c.enc.Decrypt(account.APITokenEnc)
	if err != nil {
		return nil, Upstream(err)
	}
	client := cloudflare.New(token)
	client.BaseURL = c.cfg.CFAPIBaseURL
	return client, nil
}

func (c *CF) Encrypt(plain string) (string, error) { return c.enc.Encrypt(plain) }
func (c *CF) Decrypt(enc string) (string, error)   { return c.enc.Decrypt(enc) }

// MapUpstream turns a Cloudflare API failure into an actionable AppError so the
// UI can tell "token lacks DNS:Edit" apart from "we are down".
func MapUpstream(err error) *AppError {
	if err == nil {
		return nil
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	if cloudflare.IsMalformedToken(err) {
		return NewError(422, "cf_token_malformed",
			"Cloudflare rejected the token format. Paste an API token from My Profile → API Tokens — not the Global API Key and not a tunnel token.")
	}
	if cloudflare.IsPermission(err) {
		return NewError(422, "cf_permission_denied",
			"Cloudflare rejected the request: the API token is missing a required permission (Account: Cloudflare Tunnel Edit, Zone: DNS Edit, Zone: Zone Read)")
	}
	if cloudflare.IsNotFound(err) {
		return NewError(404, "cf_not_found", "the resource no longer exists on Cloudflare")
	}
	return Upstream(err)
}

// ── Validation helpers ───────────────────────────────────────────────

var hostnameRe = regexp.MustCompile(`^(?i)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

// NormalizeHostname lowercases, trims, and strips a scheme or trailing dot that
// users paste from a browser bar.
func NormalizeHostname(raw string) (string, error) {
	h := strings.TrimSpace(strings.ToLower(raw))
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	if i := strings.IndexAny(h, "/?#"); i >= 0 {
		h = h[:i]
	}
	h = strings.TrimSuffix(h, ".")
	if h == "" {
		return "", errors.New("hostname is required")
	}
	if !hostnameRe.MatchString(h) {
		return "", fmt.Errorf("%q is not a valid hostname", raw)
	}
	return h, nil
}

var allowedSchemes = map[string]bool{
	"http": true, "https": true, "tcp": true, "ssh": true, "rdp": true,
	"smb": true, "unix": true, "unix+tls": true, "http_status": true, "hello_world": true,
}

// ValidateServiceURL checks the origin form Cloudflare accepts. It intentionally
// allows private hosts (10.x, localhost) because that is the normal case for a
// tunnel pointing at an internal service.
func ValidateServiceURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("service URL is required")
	}
	parsed, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("invalid service URL: %v", err)
	}
	if !allowedSchemes[parsed.Scheme] {
		return "", fmt.Errorf("unsupported scheme %q (use http, https, tcp, ssh, rdp, smb, unix)", parsed.Scheme)
	}
	if parsed.Scheme != "unix" && parsed.Host == "" {
		return "", errors.New("service URL needs a host, e.g. http://localhost:8080")
	}
	return s, nil
}

// MatchZone finds the most specific cached zone for a hostname. Longest suffix
// wins so "app.eu.example.com" matches "eu.example.com" over "example.com".
func MatchZone(zones []models.CFZone, hostname string) *models.CFZone {
	var best *models.CFZone
	for i := range zones {
		zone := &zones[i]
		if hostname == zone.Name || strings.HasSuffix(hostname, "."+zone.Name) {
			if best == nil || len(zone.Name) > len(best.Name) {
				best = zone
			}
		}
	}
	return best
}

func (c *CF) RequireTunnelAccess(tunnel *models.Tunnel, caller Caller) error {
	if !caller.Admin && caller.ID != tunnel.OwnerID {
		return Forbidden("you do not have access to this tunnel")
	}
	return nil
}

// Caller is the authenticated principal, decoupled from echo so the CLI and MCP
// can reuse every service later.
type Caller struct {
	ID    string
	Email string
	Admin bool
}

func (c Caller) Actor() db.Actor { return db.Actor{UserID: c.ID, Admin: c.Admin} }
