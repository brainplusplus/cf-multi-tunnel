// Package cloudflare is a small, typed wrapper around the Cloudflare API v4
// endpoints this panel needs. It deliberately exposes only what the product
// uses: token verification, accounts, zones, tunnels, tunnel configuration,
// tunnel tokens, connections and DNS records.
package cloudflare

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.cloudflare.com/client/v4"

type ErrorItem struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type APIError struct {
	StatusCode int
	Errors     []ErrorItem
}

func (e *APIError) Error() string {
	if len(e.Errors) == 0 {
		return fmt.Sprintf("cloudflare api error (status %d)", e.StatusCode)
	}
	parts := make([]string, 0, len(e.Errors))
	for _, item := range e.Errors {
		parts = append(parts, fmt.Sprintf("%d: %s", item.Code, item.Message))
	}
	return fmt.Sprintf("cloudflare api error (status %d): %s", e.StatusCode, strings.Join(parts, "; "))
}

// IsPermission reports whether the failure is a scoped-token permission gap,
// which is the most common user mistake and deserves a specific message.
func IsPermission(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.StatusCode == http.StatusForbidden || apiErr.StatusCode == http.StatusUnauthorized {
		return true
	}
	for _, item := range apiErr.Errors {
		if item.Code == 9109 || item.Code == 10000 {
			return true
		}
	}
	return false
}

func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// IsMalformedToken reports the "6003: Invalid request headers" case Cloudflare
// returns when the Authorization header does not hold a token at all (typo,
// truncated paste, or extra characters). It is a user mistake, not an outage, so
// it deserves a fixable message instead of a raw API error.
func IsMalformedToken(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	for _, item := range apiErr.Errors {
		if item.Code == 6003 || item.Code == 9103 {
			return true
		}
	}
	return false
}

type Client struct {
	Token   string
	BaseURL string
	HTTP    *http.Client
}

func New(token string) *Client {
	return &Client{
		Token:   token,
		BaseURL: defaultBaseURL,
		HTTP:    &http.Client{Timeout: 25 * time.Second},
	}
}

type envelope struct {
	Success    bool            `json:"success"`
	Errors     []ErrorItem     `json:"errors"`
	Result     json.RawMessage `json:"result"`
	ResultInfo *ResultInfo     `json:"result_info"`
}

type ResultInfo struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalPages int `json:"total_pages"`
	Count      int `json:"count"`
	TotalCount int `json:"total_count"`
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) (*ResultInfo, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("cloudflare returned non-JSON response (status %d)", resp.StatusCode)
	}
	if !env.Success || resp.StatusCode >= 400 {
		return nil, &APIError{StatusCode: resp.StatusCode, Errors: env.Errors}
	}
	if out != nil && len(env.Result) > 0 {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return nil, fmt.Errorf("decode cloudflare result: %w", err)
		}
	}
	return env.ResultInfo, nil
}

// ── Token / accounts ─────────────────────────────────────────────────

type TokenStatus struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func (c *Client) VerifyToken(ctx context.Context) (*TokenStatus, error) {
	var out TokenStatus
	if _, err := c.do(ctx, http.MethodGet, "/user/tokens/verify", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type Account struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *Client) ListAccounts(ctx context.Context) ([]Account, error) {
	var out []Account
	if _, err := c.do(ctx, http.MethodGet, "/accounts?per_page=50", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

type Zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func (c *Client) ListZones(ctx context.Context, accountID string) ([]Zone, error) {
	var out []Zone
	path := fmt.Sprintf("/zones?account.id=%s&per_page=100", url.QueryEscape(accountID))
	if _, err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ── Tunnels ──────────────────────────────────────────────────────────

type RemoteTunnel struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	ConfigSrc  string     `json:"config_src"`
	CreatedAt  time.Time  `json:"created_at"`
	DeletedAt  *time.Time `json:"deleted_at"`
	ConnActive *time.Time `json:"conns_active_at"`
}

type Connection struct {
	ID            string `json:"id"`
	ColoName      string `json:"colo_name"`
	IsPending     bool   `json:"is_pending"`
	ClientVersion string `json:"client_version"`
	OpenedAt      string `json:"opened_at"`
}

func (c *Client) ListTunnels(ctx context.Context, accountID string) ([]RemoteTunnel, error) {
	var out []RemoteTunnel
	path := fmt.Sprintf("/accounts/%s/cfd_tunnel?is_deleted=false&per_page=100", url.PathEscape(accountID))
	if _, err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetTunnel(ctx context.Context, accountID, tunnelID string) (*RemoteTunnel, error) {
	var out RemoteTunnel
	path := fmt.Sprintf("/accounts/%s/cfd_tunnel/%s", url.PathEscape(accountID), url.PathEscape(tunnelID))
	if _, err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CreateTunnel(ctx context.Context, accountID, name, configSrc string) (*RemoteTunnel, error) {
	body := map[string]any{"name": name, "config_src": configSrc}
	var out RemoteTunnel
	path := fmt.Sprintf("/accounts/%s/cfd_tunnel", url.PathEscape(accountID))
	if _, err := c.do(ctx, http.MethodPost, path, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteTunnel(ctx context.Context, accountID, tunnelID string) error {
	path := fmt.Sprintf("/accounts/%s/cfd_tunnel/%s", url.PathEscape(accountID), url.PathEscape(tunnelID))
	_, err := c.do(ctx, http.MethodDelete, path, nil, nil)
	return err
}

func (c *Client) GetTunnelToken(ctx context.Context, accountID, tunnelID string) (string, error) {
	var out string
	path := fmt.Sprintf("/accounts/%s/cfd_tunnel/%s/token", url.PathEscape(accountID), url.PathEscape(tunnelID))
	if _, err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return "", err
	}
	return out, nil
}

func (c *Client) GetConnections(ctx context.Context, accountID, tunnelID string) ([]Connection, error) {
	var out []Connection
	path := fmt.Sprintf("/accounts/%s/cfd_tunnel/%s/connections", url.PathEscape(accountID), url.PathEscape(tunnelID))
	if _, err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ── Tunnel configuration (ingress) ───────────────────────────────────

type IngressRule struct {
	Hostname      string          `json:"hostname,omitempty"`
	Path          string          `json:"path,omitempty"`
	Service       string          `json:"service"`
	OriginRequest json.RawMessage `json:"originRequest,omitempty"`
}

type TunnelConfig struct {
	Ingress       []IngressRule   `json:"ingress"`
	OriginRequest json.RawMessage `json:"originRequest,omitempty"`
}

// GetTunnelConfig reads the remote ingress list.
//
// The API wraps it: result.config.ingress. Decoding result directly into
// TunnelConfig silently yields an empty ingress, which looks exactly like a
// tunnel with no hostnames — no error, no apps, nothing to report. The wrapper
// is therefore explicit here.
func (c *Client) GetTunnelConfig(ctx context.Context, accountID, tunnelID string) (*TunnelConfig, error) {
	var body struct {
		Config TunnelConfig `json:"config"`
	}
	path := fmt.Sprintf("/accounts/%s/cfd_tunnel/%s/configurations", url.PathEscape(accountID), url.PathEscape(tunnelID))
	if _, err := c.do(ctx, http.MethodGet, path, nil, &body); err != nil {
		return nil, err
	}
	return &body.Config, nil
}

// PutTunnelConfig replaces the whole ingress list. Cloudflare has no partial
// update, which is why the panel rebuilds the full list from its own DB.
func (c *Client) PutTunnelConfig(ctx context.Context, accountID, tunnelID string, cfg TunnelConfig) error {
	path := fmt.Sprintf("/accounts/%s/cfd_tunnel/%s/configurations", url.PathEscape(accountID), url.PathEscape(tunnelID))
	_, err := c.do(ctx, http.MethodPut, path, map[string]any{"config": cfg}, nil)
	return err
}

// ── DNS ──────────────────────────────────────────────────────────────

type DNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
	TTL     int    `json:"ttl"`
}

func (c *Client) ListDNSRecords(ctx context.Context, zoneID, name string) ([]DNSRecord, error) {
	var out []DNSRecord
	path := fmt.Sprintf("/zones/%s/dns_records?name=%s&per_page=100", url.PathEscape(zoneID), url.QueryEscape(name))
	if _, err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// EnsureDNSRecord creates the CNAME or updates it when the target drifted.
// Idempotent so a retried publish never fails on "record already exists".
func (c *Client) EnsureDNSRecord(ctx context.Context, zoneID, name, content string, proxied bool) (string, error) {
	existing, err := c.ListDNSRecords(ctx, zoneID, name)
	if err != nil {
		return "", err
	}
	for _, rec := range existing {
		if rec.Type != "CNAME" {
			continue
		}
		if rec.Content == content && rec.Proxied == proxied {
			return rec.ID, nil
		}
		body := map[string]any{"type": "CNAME", "name": name, "content": content, "proxied": proxied, "ttl": 1}
		path := fmt.Sprintf("/zones/%s/dns_records/%s", url.PathEscape(zoneID), url.PathEscape(rec.ID))
		var updated DNSRecord
		if _, err := c.do(ctx, http.MethodPatch, path, body, &updated); err != nil {
			return "", err
		}
		return updated.ID, nil
	}

	body := map[string]any{"type": "CNAME", "name": name, "content": content, "proxied": proxied, "ttl": 1}
	path := fmt.Sprintf("/zones/%s/dns_records", url.PathEscape(zoneID))
	var created DNSRecord
	if _, err := c.do(ctx, http.MethodPost, path, body, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

func (c *Client) DeleteDNSRecord(ctx context.Context, zoneID, recordID string) error {
	if zoneID == "" || recordID == "" {
		return nil
	}
	path := fmt.Sprintf("/zones/%s/dns_records/%s", url.PathEscape(zoneID), url.PathEscape(recordID))
	_, err := c.do(ctx, http.MethodDelete, path, nil, nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}

// ── Tunnel token decoding ────────────────────────────────────────────

// TunnelToken is the payload inside a cloudflared token:
// base64({"a":"<account tag>","t":"<tunnel id>","s":"<tunnel secret>"}).
type TunnelToken struct {
	AccountTag   string `json:"a"`
	TunnelID     string `json:"t"`
	TunnelSecret string `json:"s"`
}

// DecodeTunnelToken accepts the token as pasted by a user. Tokens are
// base64url without padding today, but older/edge tokens may use standard
// base64, so every variant is tried before giving up.
func DecodeTunnelToken(token string) (*TunnelToken, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("tunnel token is empty")
	}
	candidates := []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	}
	var lastErr error
	for _, enc := range candidates {
		raw, err := enc.DecodeString(token)
		if err != nil {
			lastErr = err
			continue
		}
		var payload TunnelToken
		if err := json.Unmarshal(raw, &payload); err != nil {
			lastErr = err
			continue
		}
		if payload.AccountTag == "" || payload.TunnelID == "" {
			lastErr = errors.New("token is missing account tag or tunnel id")
			continue
		}
		return &payload, nil
	}
	return nil, fmt.Errorf("not a valid cloudflared tunnel token: %w", lastErr)
}
