package services

import (
	"context"
	"errors"
	"testing"

	"cf-multi-tunnel/backend/internal/models"
)

// Impersonation is the only way an admin can see what a normal user sees, so it
// must (a) never be available to a non-admin, (b) really drop admin rights
// instead of only changing the label, and (c) leave an audit trail naming the
// admin rather than blaming the user being viewed.

func seedRole(t *testing.T, svc *AuthService, email, role string, active bool) *models.User {
	t.Helper()
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	u := &models.User{Email: email, Name: email, Role: role, IsActive: active, Password: hash}
	if err := svc.users.Create(context.Background(), u); err != nil {
		t.Fatalf("create %s: %v", email, err)
	}
	return u
}

func TestImpersonateDropsAdminRights(t *testing.T) {
	svc, _, gdb := newAuthService(t)
	ctx := context.Background()

	admin := seedRole(t, svc, "admin@example.com", models.RoleAdmin, true)
	user := seedRole(t, svc, "user@example.com", models.RoleUser, true)
	caller := Caller{ID: admin.ID, Email: admin.Email, Admin: true}

	res, err := svc.Impersonate(ctx, caller, user.ID, "10.0.0.5")
	if err != nil {
		t.Fatalf("impersonate: %v", err)
	}
	if res.User.ID != user.ID {
		t.Fatalf("target = %q, want %q", res.User.ID, user.ID)
	}
	if res.OriginalUser.ID != admin.ID {
		t.Fatalf("original user = %q, want the admin", res.OriginalUser.ID)
	}
	if res.ExpiresIn <= 0 {
		t.Fatalf("impersonation token has no expiry")
	}

	claims, err := svc.ValidateAccess(res.AccessToken)
	if err != nil {
		t.Fatalf("validate impersonation token: %v", err)
	}
	if claims.UserID != user.ID {
		t.Fatalf("token acts as %q, want the target", claims.UserID)
	}
	if claims.Role != models.RoleUser {
		t.Fatalf("token role = %q; an impersonated session must not keep admin", claims.Role)
	}
	if claims.ImpersonatedBy != admin.ID {
		t.Fatalf("token does not record the admin behind it")
	}

	// The audit trail must name the admin. Without it the log says the user did
	// everything, which is worse than no log at all.
	var entry models.AuditLog
	if err := gdb.First(&entry, "action = ?", "user.impersonate_start").Error; err != nil {
		t.Fatalf("no audit entry for the impersonation: %v", err)
	}
	if entry.ActorID != admin.ID {
		t.Fatalf("audit actor = %q, want the admin who acted", entry.ActorID)
	}
	if entry.EntityID != user.ID {
		t.Fatalf("audit entity = %q, want the target", entry.EntityID)
	}
}

// A normal user must not be able to mint a token for anybody.
func TestImpersonateRequiresAdmin(t *testing.T) {
	svc, _, _ := newAuthService(t)
	ctx := context.Background()

	admin := seedRole(t, svc, "admin@example.com", models.RoleAdmin, true)
	user := seedRole(t, svc, "user@example.com", models.RoleUser, true)

	_, err := svc.Impersonate(ctx, Caller{ID: user.ID, Email: user.Email, Admin: false}, admin.ID, "")
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Status != 403 {
		t.Fatalf("non-admin impersonate = %v, want 403", err)
	}
}

func TestImpersonateRejectsSelfAndInactive(t *testing.T) {
	svc, _, gdb := newAuthService(t)
	ctx := context.Background()

	admin := seedRole(t, svc, "admin@example.com", models.RoleAdmin, true)
	gone := seedRole(t, svc, "gone@example.com", models.RoleUser, true)
	// is_active has a SQL default of true, and GORM writes the default back on
	// INSERT, so a create with IsActive:false silently becomes active. Update it.
	if err := gdb.Model(&models.User{}).Where("id = ?", gone.ID).Update("is_active", false).Error; err != nil {
		t.Fatalf("disable target: %v", err)
	}
	caller := Caller{ID: admin.ID, Email: admin.Email, Admin: true}

	if _, err := svc.Impersonate(ctx, caller, admin.ID, ""); err == nil {
		t.Fatalf("self-impersonation was allowed")
	}
	if _, err := svc.Impersonate(ctx, caller, gone.ID, ""); err == nil {
		t.Fatalf("impersonating a disabled account was allowed")
	}
	if _, err := svc.Impersonate(ctx, caller, "does-not-exist", ""); err == nil {
		t.Fatalf("impersonating a missing account was allowed")
	}
}

// An impersonated session has no refresh token, so it can never be renewed past
// its window: ending it is the browser restoring the admin's own token.
func TestImpersonationTokenCannotBeRefreshed(t *testing.T) {
	svc, _, _ := newAuthService(t)
	ctx := context.Background()

	admin := seedRole(t, svc, "admin@example.com", models.RoleAdmin, true)
	user := seedRole(t, svc, "user@example.com", models.RoleUser, true)

	res, err := svc.Impersonate(ctx, Caller{ID: admin.ID, Email: admin.Email, Admin: true}, user.ID, "")
	if err != nil {
		t.Fatalf("impersonate: %v", err)
	}
	// The result has no refresh token field at all; using the impersonation
	// token as a refresh token must fail.
	if _, err := svc.Refresh(ctx, res.AccessToken, "test", "127.0.0.1"); err == nil {
		t.Fatalf("an impersonation token was accepted as a refresh token")
	}
}
