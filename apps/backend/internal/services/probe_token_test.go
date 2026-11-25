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
)

// fakeMultiAccountCloudflare answers the two calls the probe makes, with a token
// that can see two accounts — the exact situation that used to be reported as an
// error message listing raw account ids.
func fakeMultiAccountCloudflare(t *testing.T) *httptest.Server {
	t.Helper()

	write := func(w http.ResponseWriter, status int, result any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":     status < 400,
			"errors":      []any{},
			"messages":    []any{},
			"result":      result,
			"result_info": map[string]any{"page": 1, "per_page": 100, "count": 1, "total_count": 1, "total_pages": 1},
		})
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/user/tokens/verify", func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer revoked":
			write(w, http.StatusForbidden, nil)
			return
		case "Bearer malformed":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success":  false,
				"errors":   []map[string]any{{"code": 6003, "message": "Invalid request headers"}},
				"messages": []any{},
				"result":   nil,
			})
			return
		}
		write(w, http.StatusOK, map[string]string{"id": "tok-1", "status": "active"})
	})
	mux.HandleFunc("/accounts", func(w http.ResponseWriter, r *http.Request) {
		write(w, http.StatusOK, []map[string]string{
			{"id": "acct-1", "name": "Akhmal.mib"},
			{"id": "acct-2", "name": "Bitsolusiama"},
		})
	})

	return httptest.NewServer(mux)
}

func newProbeStack(t *testing.T, apiBase string) (*AccountService, *db.AccountRepository) {
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
		CFAPIBaseURL:     apiBase,
		ConnectorMode:    "docker",
		CloudflaredImage: "cloudflare/cloudflared:latest",
		DockerBin:        "docker",
		PublicURL:        "http://192.168.0.100:7777",
		PanelInternalURL: "http://reverse-proxy:7777",
	}

	accountRepo := db.NewAccountRepository(gdb)
	cf := NewCF(cfg, cipher)
	svc := NewAccountService(accountRepo, db.NewZoneRepository(gdb), db.NewTunnelRepository(gdb),
		db.NewAppRepository(gdb), cf, NewAuditService(db.NewAuditRepository(gdb)), nil)

	return svc, accountRepo
}

// TestProbeTokenListsEveryAccountAndFlagsTheOnesAlreadyAdded drives the account
// combobox: the form needs both the choice list and enough information to grey
// out an account that is already managed.
func TestProbeTokenListsEveryAccountAndFlagsTheOnesAlreadyAdded(t *testing.T) {
	server := fakeMultiAccountCloudflare(t)
	defer server.Close()

	svc, repo := newProbeStack(t, server.URL)
	ctx := context.Background()
	caller := Caller{ID: "user-1", Email: "u@example.com"}

	// acct-1 is already managed through an API token; acct-2 is not.
	if err := repo.Create(ctx, &models.CFAccount{
		OwnerID: caller.ID, Name: "Akhmal", AccountID: "acct-1",
		Credential: "api", APITokenEnc: "x", Status: "active",
	}); err != nil {
		t.Fatalf("seed account: %v", err)
	}

	choices, err := svc.ProbeToken(ctx, caller, "good-token")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if len(choices) != 2 {
		t.Fatalf("choices = %d, want 2: %+v", len(choices), choices)
	}
	if choices[0].ID != "acct-1" || choices[0].Name != "Akhmal.mib" {
		t.Fatalf("first choice = %+v, want acct-1/Akhmal.mib", choices[0])
	}
	if !choices[0].Already {
		t.Fatal("acct-1 is already managed with an API token but was not flagged")
	}
	if choices[1].Already {
		t.Fatalf("acct-2 is new but was flagged as already added: %+v", choices[1])
	}
}

// A row created by importing a tunnel token is upgraded in place by
// CreateWithToken, so the combobox must not offer it as "already added" — that
// would hide the only way to give the account API access.
func TestProbeTokenDoesNotFlagTokenImportRowsAsAlreadyAdded(t *testing.T) {
	server := fakeMultiAccountCloudflare(t)
	defer server.Close()

	svc, repo := newProbeStack(t, server.URL)
	ctx := context.Background()
	caller := Caller{ID: "user-1", Email: "u@example.com"}

	if err := repo.Create(ctx, &models.CFAccount{
		OwnerID: caller.ID, Name: "Imported", AccountID: "acct-2",
		Credential: "token_import", Status: "active",
	}); err != nil {
		t.Fatalf("seed account: %v", err)
	}

	choices, err := svc.ProbeToken(ctx, caller, "good-token")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	for _, c := range choices {
		if c.ID == "acct-2" && c.Already {
			t.Fatal("token_import row flagged as already added; the upgrade path would be unreachable")
		}
	}
}

// An empty token must not reach Cloudflare at all.
func TestProbeTokenRejectsAnEmptyToken(t *testing.T) {
	server := fakeMultiAccountCloudflare(t)
	defer server.Close()

	svc, _ := newProbeStack(t, server.URL)

	_, err := svc.ProbeToken(context.Background(), Caller{ID: "user-1"}, "   ")
	if err == nil {
		t.Fatal("empty token accepted")
	}
	appErr, ok := err.(*AppError)
	if !ok || appErr.Code != "validation_failed" {
		t.Fatalf("err = %v, want validation_failed", err)
	}
}

// A revoked token has to surface as a real error, not as an empty combobox.
func TestProbeTokenReportsARevokedToken(t *testing.T) {
	server := fakeMultiAccountCloudflare(t)
	defer server.Close()

	svc, _ := newProbeStack(t, server.URL)

	_, err := svc.ProbeToken(context.Background(), Caller{ID: "user-1"}, "revoked")
	if err == nil {
		t.Fatal("revoked token accepted")
	}
	appErr, ok := err.(*AppError)
	if !ok || appErr.Code != "cf_permission_denied" {
		t.Fatalf("err = %v, want cf_permission_denied", err)
	}
}

// A typo or a truncated paste makes Cloudflare answer 6003 "Invalid request
// headers". The form must say "this is not a token" instead of echoing that.
func TestProbeTokenExplainsAMalformedToken(t *testing.T) {
	server := fakeMultiAccountCloudflare(t)
	defer server.Close()

	svc, _ := newProbeStack(t, server.URL)

	_, err := svc.ProbeToken(context.Background(), Caller{ID: "user-1"}, "malformed")
	if err == nil {
		t.Fatal("malformed token accepted")
	}
	appErr, ok := err.(*AppError)
	if !ok || appErr.Code != "cf_token_malformed" {
		t.Fatalf("err = %v, want cf_token_malformed", err)
	}
	if appErr.Status != 422 {
		t.Fatalf("status = %d, want 422", appErr.Status)
	}
}
