package services

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"cf-multi-tunnel/backend/internal/config"
	"cf-multi-tunnel/backend/internal/infrastructure"
	"cf-multi-tunnel/backend/internal/models"
	"cf-multi-tunnel/backend/internal/pkg/cryptox"
	db "cf-multi-tunnel/backend/internal/repositories/db"

	"gorm.io/gorm"
)

// fakeCloudflare answers exactly the endpoints the importer touches, so the
// reconcile path can be exercised without a real token or network access.
func fakeCloudflare(t *testing.T) *httptest.Server {
	t.Helper()

	ingress := map[string][]map[string]any{
		"tunnel-a": {
			{"hostname": "app.example.com", "service": "http://localhost:8080"},
			{"service": "http_status:404"},
		},
		"tunnel-b": {
			{"hostname": "panel.example.com", "service": "http://panel:7777"},
			{"service": "http_status:404"},
		},
	}
	dns := map[string][]map[string]any{
		"app.example.com": {
			{"id": "rec-1", "type": "CNAME", "name": "app.example.com", "content": "tunnel-a.cfargotunnel.com", "proxied": true, "ttl": 1},
		},
	}

	write := func(w http.ResponseWriter, result any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":     true,
			"errors":      []any{},
			"messages":    []any{},
			"result":      result,
			"result_info": map[string]any{"page": 1, "per_page": 100, "count": 1, "total_count": 1, "total_pages": 1},
		})
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/user/tokens/verify", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]string{"id": "tok-1", "status": "active"})
	})
	mux.HandleFunc("/accounts", func(w http.ResponseWriter, r *http.Request) {
		write(w, []map[string]string{{"id": "acct-1", "name": "Acme"}})
	})
	mux.HandleFunc("/zones", func(w http.ResponseWriter, r *http.Request) {
		write(w, []map[string]string{{"id": "zone-1", "name": "example.com", "status": "active"}})
	})
	mux.HandleFunc("/accounts/acct-1/cfd_tunnel", func(w http.ResponseWriter, r *http.Request) {
		write(w, []map[string]any{
			{"id": "tunnel-a", "name": "server-a", "config_src": "cloudflare"},
			{"id": "tunnel-b", "name": "laptop-b", "config_src": "cloudflare"},
		})
	})
	for id, rules := range ingress {
		id, rules := id, rules
		mux.HandleFunc("/accounts/acct-1/cfd_tunnel/"+id+"/configurations", func(w http.ResponseWriter, r *http.Request) {
			write(w, map[string]any{"ingress": rules})
		})
	}
	mux.HandleFunc("/zones/zone-1/dns_records", func(w http.ResponseWriter, r *http.Request) {
		write(w, dns[r.URL.Query().Get("name")])
	})

	return httptest.NewServer(mux)
}

func newTestStack(t *testing.T, apiBase string) (*AccountService, *TunnelService, *gorm.DB) {
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
		CFAPIBaseURL:     apiBase,
		ConnectorMode:    "docker",
		CloudflaredImage: "cloudflare/cloudflared:latest",
		DockerBin:        "docker",
		PublicURL:        "http://localhost:7777",
		PanelInternalURL: "http://reverse-proxy:7777",
	}

	accountRepo := db.NewAccountRepository(gdb)
	zoneRepo := db.NewZoneRepository(gdb)
	tunnelRepo := db.NewTunnelRepository(gdb)
	appRepo := db.NewAppRepository(gdb)
	auditSvc := NewAuditService(db.NewAuditRepository(gdb))
	cf := NewCF(cfg, cipher)

	tunnelSvc := NewTunnelService(tunnelRepo, accountRepo, zoneRepo, appRepo, cf, auditSvc, nil, cfg)
	accountSvc := NewAccountService(accountRepo, zoneRepo, tunnelRepo, appRepo, cf, auditSvc, nil)
	accountSvc.SetTunnelReconciler(tunnelSvc)

	return accountSvc, tunnelSvc, gdb
}

// TestUpgradeKeepsImportedTunnelAndImportsIngress is the exact scenario a user
// runs into: import a tunnel by token, then later add an API token for the same
// Cloudflare account. The account must be upgraded in place — not rejected as a
// duplicate — the tunnel's placement must survive, and the hostnames already
// configured on the tunnel must become published apps.
func TestUpgradeKeepsImportedTunnelAndImportsIngress(t *testing.T) {
	server := fakeCloudflare(t)
	defer server.Close()

	accountSvc, _, gdb := newTestStack(t, server.URL)
	ctx := context.Background()
	caller := Caller{ID: "user-1", Email: "u@example.com"}

	imported := &models.CFAccount{
		OwnerID:    caller.ID,
		Name:       "Imported acct-1",
		AccountID:  "acct-1",
		Credential: "token_import",
		Status:     "active",
	}
	if err := gdb.Create(imported).Error; err != nil {
		t.Fatalf("seed account: %v", err)
	}
	localTunnel := &models.Tunnel{
		OwnerID:       caller.ID,
		CFAccountID:   imported.ID,
		TunnelID:      "tunnel-a",
		Name:          "my-server",
		Credential:    "token_import",
		ConfigSrc:     "unknown",
		Location:      "local",
		HostLabel:     "this panel host",
		ConnectorMode: "docker",
		Status:        "stopped",
	}
	if err := gdb.Create(localTunnel).Error; err != nil {
		t.Fatalf("seed tunnel: %v", err)
	}

	upgraded, err := accountSvc.CreateWithToken(ctx, caller, CreateAccountInput{APIToken: "cf-token"})
	if err != nil {
		t.Fatalf("adding an API token for an imported account must upgrade it, got: %v", err)
	}
	if upgraded.ID != imported.ID {
		t.Fatalf("expected the imported account row to be reused, got a new row %s", upgraded.ID)
	}
	if upgraded.Credential != "api" {
		t.Fatalf("credential = %q, want api", upgraded.Credential)
	}

	// The tunnel the user hosted here keeps its placement; only its facts from
	// Cloudflare are refreshed.
	var kept models.Tunnel
	if err := gdb.First(&kept, "id = ?", localTunnel.ID).Error; err != nil {
		t.Fatalf("reload tunnel: %v", err)
	}
	if kept.Location != "local" || kept.HostLabel != "this panel host" || kept.ConnectorMode != "docker" {
		t.Fatalf("placement was clobbered: location=%q host=%q mode=%q", kept.Location, kept.HostLabel, kept.ConnectorMode)
	}
	if kept.ConfigSrc != "cloudflare" || kept.Credential != "api" {
		t.Fatalf("tunnel was not upgraded: config_src=%q credential=%q", kept.ConfigSrc, kept.Credential)
	}

	// The other tunnel on the account is discovered as a remote one.
	var discovered models.Tunnel
	if err := gdb.First(&discovered, "tunnel_id = ?", "tunnel-b").Error; err != nil {
		t.Fatalf("tunnel-b was not imported: %v", err)
	}
	if discovered.Location != "remote" || discovered.ConnectorMode != "external" {
		t.Fatalf("discovered tunnel has wrong placement: %q/%q", discovered.Location, discovered.ConnectorMode)
	}

	// Hostnames from the remote ingress become published apps. DNS is adopted
	// only where a matching CNAME already exists.
	var app models.PublishedApp
	if err := gdb.First(&app, "hostname = ?", "app.example.com").Error; err != nil {
		t.Fatalf("app.example.com was not imported: %v", err)
	}
	if app.TunnelID != kept.ID || app.ZoneName != "example.com" || app.ServiceURL != "http://localhost:8080" {
		t.Fatalf("imported app is wrong: %+v", app)
	}
	if !app.ManageDNS || app.DNSRecordID != "rec-1" || app.SyncStatus != "synced" {
		t.Fatalf("existing DNS record was not adopted: manage_dns=%v record=%q status=%q",
			app.ManageDNS, app.DNSRecordID, app.SyncStatus)
	}

	var panelApp models.PublishedApp
	if err := gdb.First(&panelApp, "hostname = ?", "panel.example.com").Error; err != nil {
		t.Fatalf("panel.example.com was not imported: %v", err)
	}
	if panelApp.ManageDNS || panelApp.DNSRecordID != "" {
		t.Fatalf("must not claim DNS it does not own: manage_dns=%v record=%q", panelApp.ManageDNS, panelApp.DNSRecordID)
	}

	// A second add is a genuine duplicate and must be rejected.
	if _, err := accountSvc.CreateWithToken(ctx, caller, CreateAccountInput{APIToken: "cf-token"}); err == nil {
		t.Fatal("adding the same account twice should conflict")
	}
}

// TestReconcileIsIdempotent proves the sync is an upsert: running it again on an
// unchanged account creates nothing and rewrites nothing.
func TestReconcileIsIdempotent(t *testing.T) {
	server := fakeCloudflare(t)
	defer server.Close()

	accountSvc, tunnelSvc, gdb := newTestStack(t, server.URL)
	ctx := context.Background()
	caller := Caller{ID: "user-1", Email: "u@example.com", Admin: true}

	account, err := accountSvc.CreateWithToken(ctx, caller, CreateAccountInput{APIToken: "cf-token"})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	res, err := tunnelSvc.ReconcileAccount(ctx, caller, account.ID)
	if err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if res.TunnelsCreated != 0 || res.AppsCreated != 0 || res.AppsUpdated != 0 {
		t.Fatalf("reconcile is not idempotent: %+v", res)
	}
	if res.Tunnels != 2 || res.Apps != 2 {
		t.Fatalf("unexpected totals: tunnels=%d apps=%d", res.Tunnels, res.Apps)
	}

	var tunnelCount, appCount int64
	gdb.Model(&models.Tunnel{}).Count(&tunnelCount)
	gdb.Model(&models.PublishedApp{}).Count(&appCount)
	if tunnelCount != 2 || appCount != 2 {
		t.Fatalf("duplicate rows created: tunnels=%d apps=%d", tunnelCount, appCount)
	}
}
