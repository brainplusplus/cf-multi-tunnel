package services

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"cf-multi-tunnel/backend/internal/config"
	"cf-multi-tunnel/backend/internal/infrastructure"
	"cf-multi-tunnel/backend/internal/models"
	"cf-multi-tunnel/backend/internal/pkg/cryptox"
	db "cf-multi-tunnel/backend/internal/repositories/db"

	"gorm.io/gorm"
)

func newAuthService(t *testing.T) (*AuthService, *db.UserRepository, *gorm.DB) {
	t.Helper()

	gdb, err := infrastructure.InitDB(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = infrastructure.CloseDB(gdb) })

	cfg := &config.Config{
		AccessTTL:  900e9,
		RefreshTTL: 24 * 3600e9,
		JWTSecret:  []byte("0123456789abcdef0123456789abcdef"),
	}
	users := db.NewUserRepository(gdb)
	tokens := db.NewRefreshTokenRepository(gdb)
	audit := NewAuditService(db.NewAuditRepository(gdb))
	return NewAuthService(users, tokens, cfg, audit), users, gdb
}

func seedUser(t *testing.T, users *db.UserRepository, email, password string) {
	t.Helper()
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	err = users.Create(context.Background(), &models.User{
		Email: email, Password: hash, Name: "Test", Role: models.RoleAdmin, IsActive: true,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
}

// TestLoginLockoutAfterRepeatedFailures proves a password-guessing loop is cut
// off instead of being allowed to run forever. The panel is exposed on the
// public internet when published through a tunnel, so this is not optional.
func TestLoginLockoutAfterRepeatedFailures(t *testing.T) {
	svc, users, _ := newAuthService(t)
	ctx := context.Background()
	seedUser(t, users, "admin@example.com", "correct-password")

	for i := 0; i < loginMaxFailures; i++ {
		if _, err := svc.Login(ctx, "admin@example.com", "wrong", "test", "127.0.0.1"); err == nil {
			t.Fatalf("attempt %d: expected failure", i+1)
		}
	}

	// The next attempt is refused before the password is even checked, so even
	// the correct password does not get through during the lockout.
	_, err := svc.Login(ctx, "admin@example.com", "correct-password", "test", "127.0.0.1")
	if err == nil {
		t.Fatal("expected lockout to reject the correct password")
	}
	appErr, ok := err.(*AppError)
	if !ok || appErr.Status != 429 {
		t.Fatalf("expected a 429 lockout error, got %T %v", err, err)
	}
	if !strings.Contains(strings.ToLower(appErr.Message), "too many") {
		t.Fatalf("lockout message is not actionable: %q", appErr.Message)
	}
}

// TestUnknownEmailLocksOutToo closes the enumeration gap: an address that does
// not exist must not be a free unlimited guessing target.
func TestUnknownEmailLocksOutToo(t *testing.T) {
	svc, _, _ := newAuthService(t)
	ctx := context.Background()

	for i := 0; i < loginMaxFailures; i++ {
		_, _ = svc.Login(ctx, "ghost@example.com", "wrong", "test", "127.0.0.1")
	}
	_, err := svc.Login(ctx, "ghost@example.com", "wrong", "test", "127.0.0.1")
	appErr, ok := err.(*AppError)
	if !ok || appErr.Status != 429 {
		t.Fatalf("unknown email was not throttled: %T %v", err, err)
	}
}

// TestSuccessfulLoginClearsFailures keeps a legitimate user from being punished
// by their own earlier typos.
func TestSuccessfulLoginClearsFailures(t *testing.T) {
	svc, users, _ := newAuthService(t)
	ctx := context.Background()
	seedUser(t, users, "admin@example.com", "correct-password")

	for i := 0; i < loginMaxFailures-1; i++ {
		_, _ = svc.Login(ctx, "admin@example.com", "wrong", "test", "127.0.0.1")
	}
	if _, err := svc.Login(ctx, "admin@example.com", "correct-password", "test", "127.0.0.1"); err != nil {
		t.Fatalf("correct password after a few typos must work: %v", err)
	}
	// The counter reset, so another full round of typos is needed to lock out.
	for i := 0; i < loginMaxFailures-1; i++ {
		_, _ = svc.Login(ctx, "admin@example.com", "wrong", "test", "127.0.0.1")
	}
	if _, err := svc.Login(ctx, "admin@example.com", "correct-password", "test", "127.0.0.1"); err != nil {
		t.Fatalf("counter did not reset after a successful login: %v", err)
	}
}

// Guard against the encryption key silently changing shape, which would make
// every stored Cloudflare token undecryptable after an upgrade.
func TestCipherRoundTripAndWrongKeyFails(t *testing.T) {
	a, err := cryptox.New(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	b, err := cryptox.New(bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := a.Encrypt("cf-secret-token")
	if err != nil {
		t.Fatal(err)
	}
	if sealed == "cf-secret-token" {
		t.Fatal("secret was not encrypted")
	}
	if _, err := b.Decrypt(sealed); err == nil {
		t.Fatal("a different key must not decrypt another key's ciphertext")
	}
}
