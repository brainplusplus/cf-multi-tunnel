package services

import (
	"errors"
	"testing"
)

// The panel exposure form no longer asks for the panel's own origin, and it no
// longer accepts a tunnel on another device. These tests pin that down: the
// connector container reaches the dashboard over the Docker network, and a
// connector elsewhere cannot resolve that name at all.

func TestResolvePanelServiceURLLocalUsesInternalURL(t *testing.T) {
	got, err := resolvePanelServiceURL("local", "http://reverse-proxy:7777")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "http://reverse-proxy:7777" {
		t.Fatalf("local exposure service URL = %q, want the Docker-network address", got)
	}
}

// A remote tunnel used to be accepted with a typed LAN address. That produced a
// panel which only worked while the other device happened to reach this host,
// so it is now a validation error naming the field the operator must change.
func TestResolvePanelServiceURLRejectsRemoteTunnel(t *testing.T) {
	_, err := resolvePanelServiceURL("remote", "http://reverse-proxy:7777")
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Fields["tunnel_id"] == "" {
		t.Fatalf("expected a tunnel_id validation error, got %v", err)
	}
}

func TestResolvePanelServiceURLRequiresInternalURL(t *testing.T) {
	if _, err := resolvePanelServiceURL("local", "  "); err == nil {
		t.Fatal("an empty PANEL_INTERNAL_URL must fail instead of publishing a blank service URL")
	}
}
