package services

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"cf-multi-tunnel/backend/internal/config"
	"cf-multi-tunnel/backend/internal/infrastructure"
	"cf-multi-tunnel/backend/internal/models"
	"cf-multi-tunnel/backend/internal/pkg/cryptox"
	db "cf-multi-tunnel/backend/internal/repositories/db"
)

// The edit form has to show the origin options that are already stored, and
// clearing the form has to clear them. Both directions used to be invisible:
// origin_request_json was excluded from the API response, so an edit silently
// dropped a saved noTLSVerify.

type originTestStack struct {
	apps *AppService
	app  *models.PublishedApp
}

func newOriginTestStack(t *testing.T) *originTestStack {
	t.Helper()

	gdb, err := infrastructure.InitDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = infrastructure.CloseDB(gdb) })

	cipher, err := cryptox.New(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	cfg := &config.Config{
		ConnectorMode:    "docker",
		CloudflaredImage: "cloudflare/cloudflared:latest",
		DockerBin:        "docker",
		PublicURL:        "http://192.168.0.100:7777",
		PanelInternalURL: "http://reverse-proxy:7777",
	}

	accountRepo := db.NewAccountRepository(gdb)
	zoneRepo := db.NewZoneRepository(gdb)
	tunnelRepo := db.NewTunnelRepository(gdb)
	appRepo := db.NewAppRepository(gdb)
	auditSvc := NewAuditService(db.NewAuditRepository(gdb))
	cf := NewCF(cfg, cipher)
	ctx := context.Background()

	account := &models.CFAccount{OwnerID: "u1", Name: "Acme", AccountID: "acct-1", Credential: "api"}
	if err := accountRepo.Create(ctx, account); err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := zoneRepo.ReplaceForAccount(ctx, account.ID, []models.CFZone{
		{CFAccountID: account.ID, ZoneID: "zone-1", Name: "example.com", Status: "active"},
	}); err != nil {
		t.Fatalf("cache zone: %v", err)
	}

	tunnel := &models.Tunnel{
		OwnerID: "u1", CFAccountID: account.ID, TunnelID: "tunnel-a", Name: "server-a",
		Credential: "api", ConfigSrc: "cloudflare", Location: "local", ConnectorMode: "docker",
	}
	if err := tunnelRepo.Create(ctx, tunnel); err != nil {
		t.Fatalf("create tunnel: %v", err)
	}

	app := &models.PublishedApp{
		OwnerID: "u1", TunnelID: tunnel.ID, Kind: "app", Hostname: "app.example.com",
		Path: "/*", ServiceURL: "https://192.168.0.103", ZoneID: "zone-1", ZoneName: "example.com",
		ManageDNS: true, Proxied: true, Enabled: true, SyncStatus: "synced",
	}
	if err := appRepo.Create(ctx, app); err != nil {
		t.Fatalf("create app: %v", err)
	}

	tunnelSvc := NewTunnelService(tunnelRepo, accountRepo, zoneRepo, appRepo, cf, auditSvc, nil, cfg)
	return &originTestStack{
		apps: NewAppService(appRepo, tunnelRepo, accountRepo, zoneRepo, tunnelSvc, cf, auditSvc, cfg),
		app:  app,
	}
}

// Reading an app must expose the stored origin options, otherwise the edit form
// renders an unchecked box over a saved noTLSVerify and saving turns it off.
func TestGetExposesOriginRequest(t *testing.T) {
	s := newOriginTestStack(t)
	ctx := context.Background()

	if _, err := s.apps.Update(ctx, adminCaller, s.app.ID, CreateAppInput{
		Hostname:      "app.example.com",
		ServiceURL:    "https://192.168.0.103",
		OriginRequest: json.RawMessage(`{"noTLSVerify":true}`),
	}); err != nil {
		t.Fatalf("update with origin request: %v", err)
	}

	got, err := s.apps.Get(ctx, adminCaller, s.app.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.OriginRequest) == 0 {
		t.Fatalf("origin_request was not returned to the caller")
	}
	var opts map[string]any
	if err := json.Unmarshal(got.OriginRequest, &opts); err != nil {
		t.Fatalf("origin_request is not an object: %v", err)
	}
	if opts["noTLSVerify"] != true {
		t.Fatalf("origin_request lost noTLSVerify: %s", got.OriginRequest)
	}
}

// Unchecking the box sends an empty object. That has to clear the column, not
// store "{}" and keep writing a pointless originRequest block.
func TestUpdateWithEmptyObjectClearsOriginRequest(t *testing.T) {
	s := newOriginTestStack(t)
	ctx := context.Background()

	if _, err := s.apps.Update(ctx, adminCaller, s.app.ID, CreateAppInput{
		Hostname:      "app.example.com",
		ServiceURL:    "https://192.168.0.103",
		OriginRequest: json.RawMessage(`{"noTLSVerify":true}`),
	}); err != nil {
		t.Fatalf("update with origin request: %v", err)
	}
	if _, err := s.apps.Update(ctx, adminCaller, s.app.ID, CreateAppInput{
		Hostname:      "app.example.com",
		ServiceURL:    "http://192.168.0.103",
		OriginRequest: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatalf("update with empty origin request: %v", err)
	}

	got, err := s.apps.Get(ctx, adminCaller, s.app.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.OriginRequestJSON != "" {
		t.Fatalf("origin_request_json = %q, want empty", got.OriginRequestJSON)
	}
	if len(got.OriginRequest) != 0 {
		t.Fatalf("origin_request still exposed after clearing: %s", got.OriginRequest)
	}
	if got.ServiceURL != "http://192.168.0.103" {
		t.Fatalf("service_url was not updated: %q", got.ServiceURL)
	}
}

// Editing must not require the caller to resend origin options: a nil field
// means "leave it alone", which is what a hostname-only change sends.
func TestUpdateWithoutOriginRequestKeepsIt(t *testing.T) {
	s := newOriginTestStack(t)
	ctx := context.Background()

	if _, err := s.apps.Update(ctx, adminCaller, s.app.ID, CreateAppInput{
		Hostname:      "app.example.com",
		ServiceURL:    "https://192.168.0.103",
		OriginRequest: json.RawMessage(`{"noTLSVerify":true}`),
	}); err != nil {
		t.Fatalf("update with origin request: %v", err)
	}
	if _, err := s.apps.Update(ctx, adminCaller, s.app.ID, CreateAppInput{
		Hostname:   "api.example.com",
		ServiceURL: "https://192.168.0.103",
	}); err != nil {
		t.Fatalf("update without origin request: %v", err)
	}

	got, err := s.apps.Get(ctx, adminCaller, s.app.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Hostname != "api.example.com" {
		t.Fatalf("hostname was not updated: %q", got.Hostname)
	}
	if got.OriginRequestJSON == "" {
		t.Fatalf("origin options were dropped by an unrelated edit")
	}
}
