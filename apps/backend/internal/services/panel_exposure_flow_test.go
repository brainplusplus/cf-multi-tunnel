package services

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"cf-multi-tunnel/backend/internal/config"
	"cf-multi-tunnel/backend/internal/infrastructure"
	"cf-multi-tunnel/backend/internal/models"
	"cf-multi-tunnel/backend/internal/pkg/cryptox"
	db "cf-multi-tunnel/backend/internal/repositories/db"
)

// fakePanelCloudflare records the ingress PUT so the test can assert that
// publishing the panel really rewrites the tunnel's routing table.
type fakePanelCloudflare struct {
	mu       sync.Mutex
	ingress  []byte
	dnsCalls int
}

func newFakePanelCloudflare(t *testing.T) (*fakePanelCloudflare, *httptest.Server) {
	t.Helper()
	f := &fakePanelCloudflare{}

	write := func(w http.ResponseWriter, result any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "errors": []any{}, "messages": []any{}, "result": result,
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
		write(w, []map[string]any{{"id": "tunnel-a", "name": "server-a", "config_src": "cloudflare"}})
	})
	mux.HandleFunc("/accounts/acct-1/cfd_tunnel/tunnel-a/configurations", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			f.mu.Lock()
			f.ingress = body
			f.mu.Unlock()
		}
		write(w, map[string]any{"ingress": []any{}})
	})
	mux.HandleFunc("/zones/zone-1/dns_records", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			f.mu.Lock()
			f.dnsCalls++
			f.mu.Unlock()
			write(w, map[string]any{"id": "rec-new", "type": "CNAME", "name": "dash.example.com"})
			return
		}
		write(w, []any{})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return f, srv
}

type panelFlowStack struct {
	apps     *AppService
	tunnels  *TunnelService
	tunnel   *models.Tunnel
	cfg      *config.Config
	caller   Caller
	fake     *fakePanelCloudflare
	tunnelDB *db.TunnelRepository
}

func newPanelFlowStack(t *testing.T) *panelFlowStack {
	t.Helper()

	gdb, err := infrastructure.InitDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = infrastructure.CloseDB(gdb) })

	fake, srv := newFakePanelCloudflare(t)

	cipher, err := cryptox.New(bytes.Repeat([]byte{5}, 32))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	cfg := &config.Config{
		CFAPIBaseURL:     srv.URL,
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

	tunnelSvc := NewTunnelService(tunnelRepo, accountRepo, zoneRepo, appRepo, cf, auditSvc, nil, cfg)
	accountSvc := NewAccountService(accountRepo, zoneRepo, tunnelRepo, appRepo, cf, auditSvc, nil)
	accountSvc.SetTunnelReconciler(tunnelSvc)
	appSvc := NewAppService(appRepo, tunnelRepo, accountRepo, zoneRepo, tunnelSvc, cf, auditSvc, cfg)

	ctx := context.Background()
	caller := Caller{ID: "u1", Email: "admin@example.com", Admin: true}

	account, err := accountSvc.CreateWithToken(ctx, caller, CreateAccountInput{APIToken: "tok"})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	tunnels, err := tunnelRepo.ListByAccount(ctx, account.ID)
	if err != nil || len(tunnels) == 0 {
		t.Fatalf("imported tunnels = %d, err %v", len(tunnels), err)
	}

	return &panelFlowStack{
		apps: appSvc, tunnels: tunnelSvc, tunnel: &tunnels[0],
		cfg: cfg, caller: caller, fake: fake, tunnelDB: tunnelRepo,
	}
}

// TestPublishingPanelLandsInTunnelAndAppList is the behaviour a user expects
// when they publish the dashboard: one click, and the hostname shows up in the
// tunnel's ingress and in Published apps — because it is the same row.
//
// The tunnel here is an imported one, i.e. it runs on another device, so the
// service URL must default to PUBLIC_URL (an IP:port), which that connector can
// actually reach.
func TestPublishingPanelLandsInTunnelAndAppList(t *testing.T) {
	s := newPanelFlowStack(t)
	ctx := context.Background()

	if s.tunnel.Location != "remote" {
		t.Fatalf("imported tunnel location = %q, want remote", s.tunnel.Location)
	}

	app, err := s.apps.SetPanelExposure(ctx, s.caller, PanelExposureInput{
		TunnelID: s.tunnel.ID, Subdomain: "dash", Domain: "example.com",
	})
	if err != nil {
		t.Fatalf("publish panel: %v", err)
	}

	if app.Kind != "panel" {
		t.Fatalf("kind = %q, want panel", app.Kind)
	}
	if app.Hostname != "dash.example.com" {
		t.Fatalf("hostname = %q, want dash.example.com", app.Hostname)
	}
	// Remote tunnel: PUBLIC_URL is this box's LAN IP, so no typing required.
	if app.ServiceURL != "http://192.168.0.100:7777" {
		t.Fatalf("service url = %q, want PUBLIC_URL", app.ServiceURL)
	}

	// Tunnel detail page: ListByTunnel powers the hostname table.
	byTunnel, err := s.apps.ListByTunnel(ctx, s.caller, s.tunnel.ID)
	if err != nil {
		t.Fatalf("list by tunnel: %v", err)
	}
	if !containsHost(byTunnel, "dash.example.com") {
		t.Fatalf("panel hostname missing from the tunnel's apps: %+v", byTunnel)
	}

	// Published apps page.
	all, err := s.apps.List(ctx, s.caller)
	if err != nil {
		t.Fatalf("list apps: %v", err)
	}
	if !containsHost(all, "dash.example.com") {
		t.Fatalf("panel hostname missing from the published apps list: %+v", all)
	}

	// And it is actually routed: the ingress PUT must contain the hostname.
	s.fake.mu.Lock()
	ingress := string(s.fake.ingress)
	dnsCalls := s.fake.dnsCalls
	s.fake.mu.Unlock()
	if ingress == "" {
		t.Fatal("no ingress PUT happened; the tunnel would not serve the panel")
	}
	if !bytes.Contains([]byte(ingress), []byte("dash.example.com")) {
		t.Fatalf("panel hostname missing from the ingress payload: %s", ingress)
	}
	if dnsCalls != 1 {
		t.Fatalf("dns create calls = %d, want 1", dnsCalls)
	}
}

// A tunnel hosted by this panel must proxy to the Docker-network address, not to
// PUBLIC_URL: from inside the connector container the public address is a round
// trip through Cloudflare.
func TestPublishingPanelFromLocalTunnelUsesDockerAddress(t *testing.T) {
	s := newPanelFlowStack(t)
	ctx := context.Background()

	s.tunnel.Location = "local"
	s.tunnel.ConnectorMode = "docker"
	if err := s.tunnelDB.Update(ctx, s.tunnel); err != nil {
		t.Fatalf("make tunnel local: %v", err)
	}

	app, err := s.apps.SetPanelExposure(ctx, s.caller, PanelExposureInput{
		TunnelID: s.tunnel.ID, Subdomain: "dash", Domain: "example.com",
	})
	if err != nil {
		t.Fatalf("publish panel: %v", err)
	}
	if app.ServiceURL != s.cfg.PanelInternalURL {
		t.Fatalf("service url = %q, want %q", app.ServiceURL, s.cfg.PanelInternalURL)
	}
}

func containsHost(apps []models.PublishedApp, hostname string) bool {
	for _, a := range apps {
		if a.Hostname == hostname {
			return true
		}
	}
	return false
}
