package services

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"cf-multi-tunnel/backend/internal/cloudflare"
	"cf-multi-tunnel/backend/internal/models"
)

// TestAccountUpstreamNamesTheAccount proves a failed tunnel write says which
// account failed. The panel shows four account rows in one dropdown, so a bare
// "the token is missing a permission" leaves the operator guessing which token.
func TestAccountUpstreamNamesTheAccount(t *testing.T) {
	account := &models.CFAccount{Name: "Dheryhs@gmail.com's Account"}
	err := &cloudflare.APIError{
		StatusCode: http.StatusForbidden,
		Errors:     []cloudflare.ErrorItem{{Code: 10000, Message: "Authentication error"}},
	}

	msg := AccountUpstream("create a tunnel", account, err).Message
	if !strings.Contains(msg, account.Name) {
		t.Fatalf("message must name the account, got %q", msg)
	}
	if !strings.Contains(msg, "10000") {
		t.Fatalf("message must carry Cloudflare's code so the fix is findable, got %q", msg)
	}
	// 10000 is the "account not in the token's Include list" case, not a plain
	// missing scope; the message has to point at the policy include.
	if !strings.Contains(msg, "Include") {
		t.Fatalf("10000 must explain the Account policy include, got %q", msg)
	}
}

// TestAccountUpstreamKeepsGenericPermissionMessage covers a 403 that carries no
// Cloudflare error item (a proxy or a bare forbidden): the account name is still
// prepended and no bogus code is invented.
func TestAccountUpstreamKeepsGenericPermissionMessage(t *testing.T) {
	account := &models.CFAccount{Name: "Acme"}
	err := &cloudflare.APIError{StatusCode: http.StatusForbidden}

	appErr := AccountUpstream("read the tunnel token", account, err)
	if appErr.Status != http.StatusUnprocessableEntity || appErr.Code != "cf_permission_denied" {
		t.Fatalf("want 422 cf_permission_denied, got %d %s", appErr.Status, appErr.Code)
	}
	if !strings.Contains(appErr.Message, "read the tunnel token") || !strings.Contains(appErr.Message, "Acme") {
		t.Fatalf("message must name the action and account, got %q", appErr.Message)
	}
}

// TestMapUpstreamPermissionMentionsTheRequiredScopes keeps the generic message
// actionable: it is what a fresh operator reads when they never created a token.
func TestMapUpstreamPermissionMentionsTheRequiredScopes(t *testing.T) {
	err := &cloudflare.APIError{
		StatusCode: http.StatusForbidden,
		Errors:     []cloudflare.ErrorItem{{Code: 9109, Message: "Unauthorized to access requested resource"}},
	}
	appErr := MapUpstream(err)
	if appErr.Status != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", appErr.Status)
	}
	for _, part := range []string{"Cloudflare Tunnel", "DNS", "Zone"} {
		if !strings.Contains(appErr.Message, part) {
			t.Fatalf("message must mention %q, got %q", part, appErr.Message)
		}
	}
}

// TestMapUpstreamPassesAppErrorsThrough makes sure an error the service already
// shaped is never re-wrapped into a 502.
func TestMapUpstreamPassesAppErrorsThrough(t *testing.T) {
	original := Conflict("already exists")
	if got := MapUpstream(original); got != original {
		t.Fatalf("MapUpstream rewrapped an AppError: %v", got)
	}
	if got := MapUpstream(errors.New("boom")); got.Status != http.StatusBadGateway {
		t.Fatalf("plain errors should be 502, got %d", got.Status)
	}
}
