package services

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"cf-multi-tunnel/backend/internal/config"
	"cf-multi-tunnel/backend/internal/infrastructure"
	"cf-multi-tunnel/backend/internal/models"
	"cf-multi-tunnel/backend/internal/pkg/cryptox"
	db "cf-multi-tunnel/backend/internal/repositories/db"
)

// The panel exposure is stored as a published_app row with kind="panel" so that
// one table drives both the tunnel ingress and the Published apps list. That
// sharing only works if every write path respects the flag, which is what these
// tests pin down.

type panelTestStack struct {
	apps    *AppService
	tunnels *TunnelService
	tunnel  *models.Tunnel
	app     *models.PublishedApp
}

func newPanelTestStack(t *testing.T) *panelTestStack {
	t.Helper()

	gdb, err := infrastructure.InitDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = infrastructure.CloseDB(gdb) })

	cipher, err := cryptox.New(bytes.Repeat([]byte{9}, 32))
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

	account := &models.CFAccount{OwnerID: "u1", Name: "Acme", AccountID: "acct-1", Credential: "api"}
	if err := accountRepo.Create(context.Background(), account); err != nil {
		t.Fatalf("create account: %v", err)
	}
	tunnel := &models.Tunnel{
		OwnerID: "u1", CFAccountID: account.ID, TunnelID: "tunnel-a", Name: "server-a",
		Credential: "api", ConfigSrc: "cloudflare", Location: "local", ConnectorMode: "docker",
	}
	if err := tunnelRepo.Create(context.Background(), tunnel); err != nil {
		t.Fatalf("create tunnel: %v", err)
	}

	app := &models.PublishedApp{
		OwnerID: "u1", TunnelID: tunnel.ID, Kind: "panel", Hostname: "panel.example.com",
		Path: "/*", ServiceURL: "http://reverse-proxy:7777", ZoneID: "zone-1", ZoneName: "example.com",
		ManageDNS: true, Proxied: true, Enabled: true, SyncStatus: "synced",
	}
	if err := appRepo.Create(context.Background(), app); err != nil {
		t.Fatalf("create panel app: %v", err)
	}

	tunnelSvc := NewTunnelService(tunnelRepo, accountRepo, zoneRepo, appRepo, cf, auditSvc, nil, cfg)
	appSvc := NewAppService(appRepo, tunnelRepo, accountRepo, zoneRepo, tunnelSvc, cf, auditSvc, cfg)

	return &panelTestStack{apps: appSvc, tunnels: tunnelSvc, tunnel: tunnel, app: app}
}

var adminCaller = Caller{ID: "u1", Email: "admin@example.com", Admin: true}

func TestPanelRowCannotBeDeletedFromTheAppList(t *testing.T) {
	s := newPanelTestStack(t)
	err := s.apps.Delete(context.Background(), adminCaller, s.app.ID)
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Status != 403 {
		t.Fatalf("expected 403 for deleting the panel row, got %v", err)
	}
}

func TestPanelRowCannotBeEditedFromTheAppList(t *testing.T) {
	s := newPanelTestStack(t)
	_, err := s.apps.Update(context.Background(), adminCaller, s.app.ID, CreateAppInput{
		Hostname: "evil.example.com", ServiceURL: "http://localhost:1",
	})
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Status != 403 {
		t.Fatalf("expected 403 for editing the panel row, got %v", err)
	}
}

func TestPanelRowCannotBeDisabledFromTheAppList(t *testing.T) {
	s := newPanelTestStack(t)
	_, err := s.apps.SetEnabled(context.Background(), adminCaller, s.app.ID, false)
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Status != 403 {
		t.Fatalf("expected 403 for disabling the panel row, got %v", err)
	}
}

// Deleting the tunnel cascades to its published_app rows, so without a guard the
// panel exposure would disappear while Settings still showed it as published.
func TestTunnelDeleteRefusesToDropThePanelExposure(t *testing.T) {
	s := newPanelTestStack(t)
	err := s.tunnels.Delete(context.Background(), adminCaller, s.tunnel.ID)
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Status != 409 {
		t.Fatalf("expected 409 for deleting the panel's tunnel, got %v", err)
	}
}
