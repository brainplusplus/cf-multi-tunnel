package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"cf-multi-tunnel/backend/internal/cloudflare"
	"cf-multi-tunnel/backend/internal/config"
	"cf-multi-tunnel/backend/internal/infrastructure"
	"cf-multi-tunnel/backend/internal/models"
	"cf-multi-tunnel/backend/internal/pkg/cryptox"
	db "cf-multi-tunnel/backend/internal/repositories/db"

	"gorm.io/gorm"
)

// dnsFake answers only the DNS endpoints the page touches and records what the
// service sent, so a test can assert both the request shape and that a refused
// write never reached Cloudflare.
type dnsFake struct {
	mu      sync.Mutex
	records []cloudflare.DNSRecord
	created []cloudflare.DNSRecordInput
	updated []cloudflare.DNSRecordInput
	deletes int
}

func (f *dnsFake) server(t *testing.T) *httptest.Server {
	t.Helper()

	write := func(w http.ResponseWriter, result any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":  true,
			"errors":   []any{},
			"messages": []any{},
			"result":   result,
			"result_info": map[string]any{
				"page": 1, "per_page": 100, "count": 1, "total_count": 1, "total_pages": 1,
			},
		})
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/zones", func(w http.ResponseWriter, r *http.Request) {
		write(w, []map[string]string{{"id": "zone-1", "name": "example.com", "status": "active"}})
	})
	mux.HandleFunc("/zones/zone-1/dns_records", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			f.mu.Lock()
			defer f.mu.Unlock()
			write(w, f.records)
		case http.MethodPost:
			var in cloudflare.DNSRecordInput
			_ = json.NewDecoder(r.Body).Decode(&in)
			f.mu.Lock()
			f.created = append(f.created, in)
			f.mu.Unlock()
			proxied := in.Proxied != nil && *in.Proxied
			write(w, cloudflare.DNSRecord{
				ID: "rec-new", Type: in.Type, Name: in.Name, Content: in.Content,
				Proxied: proxied, TTL: in.TTL, Priority: in.Priority,
			})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/zones/zone-1/dns_records/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/zones/zone-1/dns_records/")
		switch r.Method {
		case http.MethodPatch:
			var in cloudflare.DNSRecordInput
			_ = json.NewDecoder(r.Body).Decode(&in)
			f.mu.Lock()
			f.updated = append(f.updated, in)
			f.mu.Unlock()
			write(w, cloudflare.DNSRecord{ID: id, Type: in.Type, Name: in.Name, Content: in.Content})
		case http.MethodDelete:
			f.mu.Lock()
			f.deletes++
			f.mu.Unlock()
			write(w, map[string]string{"id": id})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	return httptest.NewServer(mux)
}

func newDNSStack(t *testing.T, apiBase string) (*DNSService, *gorm.DB, *cryptox.Cipher) {
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

	cfg := &config.Config{CFAPIBaseURL: apiBase}
	accountRepo := db.NewAccountRepository(gdb)
	zoneRepo := db.NewZoneRepository(gdb)
	tunnelRepo := db.NewTunnelRepository(gdb)
	appRepo := db.NewAppRepository(gdb)
	auditSvc := NewAuditService(db.NewAuditRepository(gdb))
	cf := NewCF(cfg, cipher)
	accountSvc := NewAccountService(accountRepo, zoneRepo, tunnelRepo, appRepo, cf, auditSvc, nil)
	svc := NewDNSService(accountRepo, zoneRepo, tunnelRepo, appRepo, accountSvc, cf, auditSvc)
	return svc, gdb, cipher
}

func seedDNSAccount(t *testing.T, gdb *gorm.DB, cipher *cryptox.Cipher, ownerID string) *models.CFAccount {
	t.Helper()

	enc, err := cipher.Encrypt("cf-token")
	if err != nil {
		t.Fatalf("encrypt token: %v", err)
	}
	account := &models.CFAccount{
		OwnerID: ownerID, Name: "Acme", AccountID: "acct-1",
		Credential: "api", APITokenEnc: enc, Status: "active",
	}
	if err := gdb.Create(account).Error; err != nil {
		t.Fatalf("seed account: %v", err)
	}
	return account
}

func seedDNSZone(t *testing.T, gdb *gorm.DB, accountID string) *models.CFZone {
	t.Helper()

	zone := &models.CFZone{CFAccountID: accountID, ZoneID: "zone-1", Name: "example.com", Status: "active"}
	if err := gdb.Create(zone).Error; err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	return zone
}

func boolPtr(v bool) *bool { return &v }

// TestDNSRecordsAnnotateTunnelApp is the row the Cloudflare dashboard labels
// "Tunnel": a CNAME to cfargotunnel.com that a published app owns. The panel
// must expose both facts so the UI can link the tunnel and lock the row.
func TestDNSRecordsAnnotateTunnelApp(t *testing.T) {
	fake := &dnsFake{records: []cloudflare.DNSRecord{
		{ID: "rec-1", Type: "CNAME", Name: "app.example.com", Content: "tunnel-a.cfargotunnel.com", Proxied: true, TTL: 1},
		{ID: "rec-2", Type: "TXT", Name: "app.example.com", Content: "hello", TTL: 1},
	}}
	server := fake.server(t)
	defer server.Close()

	svc, gdb, cipher := newDNSStack(t, server.URL)
	account := seedDNSAccount(t, gdb, cipher, "owner-1")
	zone := seedDNSZone(t, gdb, account.ID)

	tunnel := &models.Tunnel{
		OwnerID: "owner-1", CFAccountID: account.ID, TunnelID: "tunnel-a", Name: "server-a",
		Credential: "api", ConfigSrc: "cloudflare", Location: "local", ConnectorMode: "docker",
	}
	if err := gdb.Create(tunnel).Error; err != nil {
		t.Fatalf("seed tunnel: %v", err)
	}
	app := &models.PublishedApp{
		OwnerID: "owner-1", TunnelID: tunnel.ID, Kind: "app", Hostname: "app.example.com",
		ZoneID: "zone-1", ZoneName: "example.com", DNSRecordID: "rec-1",
		ServiceURL: "http://192.168.0.103:80", ManageDNS: true, Proxied: true, Enabled: true,
	}
	if err := gdb.Create(app).Error; err != nil {
		t.Fatalf("seed app: %v", err)
	}

	views, truncated, err := svc.ListRecords(context.Background(), Caller{ID: "owner-1"}, zone.ID)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if truncated {
		t.Fatalf("one page of records must not be reported as truncated")
	}
	if len(views) != 2 {
		t.Fatalf("got %d views, want 2", len(views))
	}

	tunnelRow := views[0]
	if tunnelRow.DisplayType != "Tunnel" {
		t.Fatalf("display type = %q, want Tunnel", tunnelRow.DisplayType)
	}
	if !tunnelRow.Managed || tunnelRow.AppID != app.ID {
		t.Fatalf("row not linked to its app: managed=%v app=%q", tunnelRow.Managed, tunnelRow.AppID)
	}
	if tunnelRow.TunnelName != "server-a" || tunnelRow.TunnelID != "tunnel-a" {
		t.Fatalf("row not linked to its tunnel: id=%q name=%q", tunnelRow.TunnelID, tunnelRow.TunnelName)
	}
	if views[1].Managed || views[1].DisplayType != "TXT" {
		t.Fatalf("plain TXT must stay editable: managed=%v type=%q", views[1].Managed, views[1].DisplayType)
	}
}

// TestDNSDeleteRefusesTunnelManagedRecord proves the lock is enforced before
// the API call, not just hidden in the UI.
func TestDNSDeleteRefusesTunnelManagedRecord(t *testing.T) {
	fake := &dnsFake{records: []cloudflare.DNSRecord{
		{ID: "rec-1", Type: "CNAME", Name: "app.example.com", Content: "tunnel-a.cfargotunnel.com", Proxied: true, TTL: 1},
	}}
	server := fake.server(t)
	defer server.Close()

	svc, gdb, cipher := newDNSStack(t, server.URL)
	account := seedDNSAccount(t, gdb, cipher, "owner-1")
	zone := seedDNSZone(t, gdb, account.ID)
	app := &models.PublishedApp{
		OwnerID: "owner-1", TunnelID: "tunnel-row", Kind: "app", Hostname: "app.example.com",
		ZoneID: "zone-1", DNSRecordID: "rec-1", ServiceURL: "http://x", ManageDNS: true, Proxied: true, Enabled: true,
	}
	if err := gdb.Create(app).Error; err != nil {
		t.Fatalf("seed app: %v", err)
	}

	err := svc.DeleteRecord(context.Background(), Caller{ID: "owner-1"}, zone.ID, "rec-1")
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Status != http.StatusConflict {
		t.Fatalf("want 409 conflict, got %v", err)
	}
	if fake.deletes != 0 {
		t.Fatalf("a managed record must never reach the delete endpoint (calls=%d)", fake.deletes)
	}
	if !strings.Contains(appErr.Message, app.Hostname) {
		t.Fatalf("conflict must name the owning app, got %q", appErr.Message)
	}
}

// TestDNSUpdateRefusesTunnelManagedRecord keeps the lock symmetric: hiding the
// Edit button is not enough, because the API would otherwise rename a CNAME out
// from under the tunnel that depends on it.
func TestDNSUpdateRefusesTunnelManagedRecord(t *testing.T) {
	fake := &dnsFake{records: []cloudflare.DNSRecord{
		{ID: "rec-1", Type: "CNAME", Name: "app.example.com", Content: "tunnel-a.cfargotunnel.com", Proxied: true, TTL: 1},
	}}
	server := fake.server(t)
	defer server.Close()

	svc, gdb, cipher := newDNSStack(t, server.URL)
	account := seedDNSAccount(t, gdb, cipher, "owner-1")
	zone := seedDNSZone(t, gdb, account.ID)
	app := &models.PublishedApp{
		OwnerID: "owner-1", TunnelID: "tunnel-row", Kind: "app", Hostname: "app.example.com",
		ZoneID: "zone-1", DNSRecordID: "rec-1", ServiceURL: "http://x", ManageDNS: true, Proxied: true, Enabled: true,
	}
	if err := gdb.Create(app).Error; err != nil {
		t.Fatalf("seed app: %v", err)
	}

	_, err := svc.UpdateRecord(context.Background(), Caller{ID: "owner-1"}, zone.ID, "rec-1", DNSRecordInput{
		Type: "CNAME", Name: "app", Content: "elsewhere.example.net",
	})
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Status != http.StatusConflict {
		t.Fatalf("want 409 conflict, got %v", err)
	}
	if len(fake.updated) != 0 {
		t.Fatalf("a managed record must never reach the update endpoint (calls=%d)", len(fake.updated))
	}
}

// TestDNSCreateRefusesPublishedHostname stops two owners for one name: adding a
// second CNAME for a hostname an app publishes would fight the tunnel's ingress.
func TestDNSCreateRefusesPublishedHostname(t *testing.T) {
	fake := &dnsFake{}
	server := fake.server(t)
	defer server.Close()

	svc, gdb, cipher := newDNSStack(t, server.URL)
	account := seedDNSAccount(t, gdb, cipher, "owner-1")
	zone := seedDNSZone(t, gdb, account.ID)
	app := &models.PublishedApp{
		OwnerID: "owner-1", TunnelID: "tunnel-row", Kind: "app", Hostname: "app.example.com",
		ZoneID: "zone-1", ServiceURL: "http://x", ManageDNS: true, Proxied: true, Enabled: true,
	}
	if err := gdb.Create(app).Error; err != nil {
		t.Fatalf("seed app: %v", err)
	}

	_, err := svc.CreateRecord(context.Background(), Caller{ID: "owner-1"}, zone.ID, DNSRecordInput{
		Type: "CNAME", Name: "app", Content: "elsewhere.example.net",
	})
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Status != http.StatusConflict {
		t.Fatalf("want 409 conflict, got %v", err)
	}
	if len(fake.created) != 0 {
		t.Fatalf("the conflicting CNAME must not reach Cloudflare (calls=%d)", len(fake.created))
	}
}

func TestDNSCreateNormalizesRelativeName(t *testing.T) {
	fake := &dnsFake{}
	server := fake.server(t)
	defer server.Close()

	svc, gdb, cipher := newDNSStack(t, server.URL)
	account := seedDNSAccount(t, gdb, cipher, "owner-1")
	zone := seedDNSZone(t, gdb, account.ID)

	view, err := svc.CreateRecord(context.Background(), Caller{ID: "owner-1"}, zone.ID, DNSRecordInput{
		Type: "A", Name: "www", Content: "203.0.113.10", Comment: "web",
	})
	if err != nil {
		t.Fatalf("create record: %v", err)
	}
	if view.Name != "www.example.com" {
		t.Fatalf("response name = %q, want www.example.com", view.Name)
	}
	if len(fake.created) != 1 {
		t.Fatalf("expected one create call, got %d", len(fake.created))
	}
	sent := fake.created[0]
	if sent.Name != "www.example.com" {
		t.Fatalf("sent name = %q, want the absolute name", sent.Name)
	}
	if sent.Proxied == nil || !*sent.Proxied {
		t.Fatalf("A records should default to proxied")
	}
}

func TestDNSCreateValidatesFields(t *testing.T) {
	fake := &dnsFake{}
	server := fake.server(t)
	defer server.Close()

	svc, gdb, cipher := newDNSStack(t, server.URL)
	account := seedDNSAccount(t, gdb, cipher, "owner-1")
	zone := seedDNSZone(t, gdb, account.ID)
	ctx := context.Background()

	cases := []struct {
		name  string
		in    DNSRecordInput
		field string
	}{
		{"bad name", DNSRecordInput{Type: "A", Name: "bad name", Content: "203.0.113.10"}, "name"},
		{"ipv6 in A", DNSRecordInput{Type: "A", Name: "v6", Content: "2001:db8::1"}, "content"},
		{"ipv4 in AAAA", DNSRecordInput{Type: "AAAA", Name: "v4", Content: "203.0.113.10"}, "content"},
		{"proxied TXT", DNSRecordInput{Type: "TXT", Name: "txt", Content: "hello", Proxied: boolPtr(true)}, "proxied"},
		{"srv without target", DNSRecordInput{Type: "SRV", Service: "_sip", Protocol: "_tcp"}, "target"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateRecord(ctx, Caller{ID: "owner-1"}, zone.ID, tc.in)
			var appErr *AppError
			if !errors.As(err, &appErr) || appErr.Status != http.StatusUnprocessableEntity {
				t.Fatalf("want 422, got %v", err)
			}
			if appErr.Fields[tc.field] == "" {
				t.Fatalf("expected a %q field error, got %v", tc.field, appErr.Fields)
			}
		})
	}
	if len(fake.created) != 0 {
		t.Fatalf("invalid records must not reach Cloudflare (calls=%d)", len(fake.created))
	}
}

func TestDNSRecordsAreOwnerScoped(t *testing.T) {
	fake := &dnsFake{}
	server := fake.server(t)
	defer server.Close()

	svc, gdb, cipher := newDNSStack(t, server.URL)
	account := seedDNSAccount(t, gdb, cipher, "owner-1")
	zone := seedDNSZone(t, gdb, account.ID)

	_, _, err := svc.ListRecords(context.Background(), Caller{ID: "someone-else"}, zone.ID)
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Status != http.StatusForbidden {
		t.Fatalf("want 403 for another owner's zone, got %v", err)
	}
}

func TestDNSSyncZonesCountsAccounts(t *testing.T) {
	fake := &dnsFake{}
	server := fake.server(t)
	defer server.Close()

	svc, gdb, cipher := newDNSStack(t, server.URL)
	seedDNSAccount(t, gdb, cipher, "owner-1")

	result, err := svc.SyncZones(context.Background(), Caller{ID: "owner-1"})
	if err != nil {
		t.Fatalf("sync zones: %v", err)
	}
	if result.Accounts != 1 || result.Zones != 1 || len(result.Failed) != 0 {
		t.Fatalf("unexpected sync result: %+v", result)
	}

	var zones []models.CFZone
	if err := gdb.Find(&zones).Error; err != nil {
		t.Fatalf("reload zones: %v", err)
	}
	if len(zones) != 1 || zones[0].ZoneID != "zone-1" {
		t.Fatalf("zones were not cached: %+v", zones)
	}
}
