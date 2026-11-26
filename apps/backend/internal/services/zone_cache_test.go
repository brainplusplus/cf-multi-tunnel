package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"cf-multi-tunnel/backend/internal/models"
)

// zoneCacheServer answers the handful of endpoints CreateWithToken touches and
// lets a test grow the zone list after the panel has cached it, which is exactly
// what happens when an operator adds a domain in Cloudflare.
type zoneCacheServer struct {
	*httptest.Server
	mu        sync.Mutex
	zoneList  []map[string]string
	zoneCalls int
}

func newZoneCacheServer(t *testing.T) *zoneCacheServer {
	t.Helper()
	s := &zoneCacheServer{
		zoneList: []map[string]string{{"id": "zone-1", "name": "example.com", "status": "active"}},
	}
	mux := http.NewServeMux()
	respond := func(w http.ResponseWriter, result any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":     true,
			"errors":      []any{},
			"messages":    []any{},
			"result":      result,
			"result_info": map[string]any{"page": 1, "per_page": 100, "count": 1, "total_count": 1, "total_pages": 1},
		})
	}
	mux.HandleFunc("/user/tokens/verify", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]string{"id": "tok-1", "status": "active"})
	})
	mux.HandleFunc("/accounts", func(w http.ResponseWriter, r *http.Request) {
		respond(w, []map[string]string{{"id": "acct-1", "name": "Acme"}})
	})
	mux.HandleFunc("/zones", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.zoneCalls++
		out := append([]map[string]string(nil), s.zoneList...)
		s.mu.Unlock()
		respond(w, out)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		respond(w, []any{})
	})
	s.Server = httptest.NewServer(mux)
	return s
}

func (s *zoneCacheServer) addZone(id, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.zoneList = append(s.zoneList, map[string]string{"id": id, "name": name, "status": "active"})
}

func (s *zoneCacheServer) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.zoneCalls
}

// TestListZonesRefreshesStaleCache is the operator-visible bug: a zone created
// in Cloudflare after the last Sync never showed up in the publish-app DOMAIN
// dropdown, because ListZones returned the cache unconditionally. A stale cache
// must refresh itself on read instead of demanding a manual Sync.
func TestListZonesRefreshesStaleCache(t *testing.T) {
	server := newZoneCacheServer(t)
	defer server.Close()

	accountSvc, _, gdb := newTestStack(t, server.URL)
	ctx := context.Background()
	caller := Caller{ID: "user-1", Email: "u@example.com"}

	account, err := accountSvc.CreateWithToken(ctx, caller, CreateAccountInput{APIToken: "cf-token"})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// The cache was written before the operator added the new zone, so it is
	// both incomplete and older than the TTL.
	if err := gdb.Model(&models.CFZone{}).Where("cf_account_id = ?", account.ID).
		Update("synced_at", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatalf("age zone cache: %v", err)
	}
	server.addZone("zone-2", "new.example")

	zones, err := accountSvc.ListZones(ctx, caller, account.ID)
	if err != nil {
		t.Fatalf("list zones: %v", err)
	}
	names := map[string]bool{}
	for _, z := range zones {
		names[z.Name] = true
	}
	if !names["new.example"] || !names["example.com"] {
		t.Fatalf("stale cache was not refreshed, got %v", names)
	}
}

// TestListZonesServesFreshCacheWithoutNetwork keeps the original intent: the
// cache exists so a page load does not hit Cloudflare every time. A fresh cache
// must be served as-is, even when the API is unreachable.
func TestListZonesServesFreshCacheWithoutNetwork(t *testing.T) {
	server := newZoneCacheServer(t)

	accountSvc, _, _ := newTestStack(t, server.URL)
	ctx := context.Background()
	caller := Caller{ID: "user-1", Email: "u@example.com"}

	account, err := accountSvc.CreateWithToken(ctx, caller, CreateAccountInput{APIToken: "cf-token"})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	before := server.calls()
	server.Close()

	zones, err := accountSvc.ListZones(ctx, caller, account.ID)
	if err != nil {
		t.Fatalf("fresh cache must not require the network, got: %v", err)
	}
	if len(zones) != 1 || zones[0].Name != "example.com" {
		t.Fatalf("unexpected cached zones: %+v", zones)
	}
	if after := server.calls(); after != before {
		t.Fatalf("fresh cache triggered %d extra Cloudflare call(s)", after-before)
	}
}
