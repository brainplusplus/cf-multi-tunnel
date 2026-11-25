package services

import (
	"errors"
	"testing"
)

// The panel exposure form no longer asks for the panel's own origin: the server
// derives it. These tests pin down both branches, because getting it wrong is
// either a broken dashboard (local) or an infinite loop (remote).

func TestResolvePanelServiceURLLocalUsesInternalURL(t *testing.T) {
	got, err := resolvePanelServiceURL(PanelExposureInput{}, "local", "http://192.168.0.100:7777", "http://reverse-proxy:7777")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "http://reverse-proxy:7777" {
		t.Fatalf("local exposure service URL = %q, want the Docker-network address", got)
	}
}

// A single-host install sets PUBLIC_URL to its LAN IP. That is also the only
// address a connector on another device can reach, so it must be reused instead
// of asking the operator to retype it.
func TestResolvePanelServiceURLRemoteDefaultsToPublicIP(t *testing.T) {
	got, err := resolvePanelServiceURL(PanelExposureInput{}, "remote", "http://192.168.0.100:7777", "http://reverse-proxy:7777")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "http://192.168.0.100:7777" {
		t.Fatalf("remote exposure service URL = %q, want the PUBLIC_URL IP:port", got)
	}
}

// When PUBLIC_URL is a DNS name it is served by the tunnel itself, so reusing it
// would loop. The operator has to supply a reachable address.
func TestResolvePanelServiceURLRemoteRequiresAddressForHostnamePublicURL(t *testing.T) {
	_, err := resolvePanelServiceURL(
		PanelExposureInput{Subdomain: "panel", Domain: "example.com"},
		"remote", "https://panel.example.com", "http://reverse-proxy:7777",
	)
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Fields["service_url"] == "" {
		t.Fatalf("expected a service_url validation error, got %v", err)
	}
}

func TestResolvePanelServiceURLRemoteKeepsExplicitValue(t *testing.T) {
	got, err := resolvePanelServiceURL(
		PanelExposureInput{Subdomain: "panel", Domain: "example.com", ServiceURL: "http://10.0.0.5:7777"},
		"remote", "https://panel.example.com", "http://reverse-proxy:7777",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "http://10.0.0.5:7777" {
		t.Fatalf("service URL = %q, want the explicit value", got)
	}
}

// The old check rejected any service URL whose host matched PUBLIC_URL, which
// broke the normal "PUBLIC_URL is this box's LAN IP" install. An IP literal
// cannot be a Cloudflare hostname, so there is nothing to loop through.
func TestCheckNoLoopAllowsPanelOwnIP(t *testing.T) {
	in := PanelExposureInput{Subdomain: "panel", Domain: "example.com"}
	if err := checkNoLoop("http://192.168.0.100:7777", in, "http://192.168.0.100:7777"); err != nil {
		t.Fatalf("IP service URL should be allowed, got %v", err)
	}
}

func TestCheckNoLoopRejectsPublishedHostname(t *testing.T) {
	in := PanelExposureInput{Subdomain: "panel", Domain: "example.com"}
	if err := checkNoLoop("https://panel.example.com", in, "http://192.168.0.100:7777"); err == nil {
		t.Fatal("service URL equal to the published hostname must be rejected")
	}
}

func TestCheckNoLoopRejectsPublicHostname(t *testing.T) {
	in := PanelExposureInput{Subdomain: "dash", Domain: "example.com"}
	if err := checkNoLoop("https://panel.example.com", in, "https://panel.example.com"); err == nil {
		t.Fatal("service URL equal to a DNS PUBLIC_URL must be rejected")
	}
}

func TestRemotePanelServiceURLOnlyAcceptsIP(t *testing.T) {
	cases := []struct{ in, want string }{
		{"http://192.168.0.100:7777", "http://192.168.0.100:7777"},
		{"http://192.168.0.100", "http://192.168.0.100"},
		{"https://panel.example.com", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := remotePanelServiceURL(c.in); got != c.want {
			t.Errorf("remotePanelServiceURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
