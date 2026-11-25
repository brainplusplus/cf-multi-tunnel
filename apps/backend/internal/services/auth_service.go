package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"cf-multi-tunnel/backend/internal/config"
	"cf-multi-tunnel/backend/internal/models"
	db "cf-multi-tunnel/backend/internal/repositories/db"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	users  *db.UserRepository
	tokens *db.RefreshTokenRepository
	cfg    *config.Config
	audit  *AuditService

	// In-memory login throttle. Keyed by the submitted email, not the client IP,
	// because the panel usually sits behind the reverse proxy (one IP) or a
	// Cloudflare tunnel. A per-email counter therefore protects the account that
	// is actually under attack regardless of how the request arrived.
	attempts  map[string]*loginAttempt
	attemptMu sync.Mutex
}

// Tunable so a test can exercise lockout without waiting ten minutes.
const (
	loginMaxFailures = 5
	loginWindow      = 15 * time.Minute
	loginLockout     = 10 * time.Minute
)

type loginAttempt struct {
	failures int
	first    time.Time
	lockedTo time.Time
}

func NewAuthService(users *db.UserRepository, tokens *db.RefreshTokenRepository, cfg *config.Config, audit *AuditService) *AuthService {
	return &AuthService{
		users: users, tokens: tokens, cfg: cfg, audit: audit,
		attempts: make(map[string]*loginAttempt),
	}
}

// lockoutRemaining reports how long this email is still blocked, if at all.
func (s *AuthService) lockoutRemaining(email string) time.Duration {
	s.attemptMu.Lock()
	defer s.attemptMu.Unlock()
	rec := s.attempts[email]
	if rec == nil {
		return 0
	}
	if wait := time.Until(rec.lockedTo); wait > 0 {
		return wait
	}
	// Expire stale counters so the map does not grow without bound.
	if time.Since(rec.first) > loginWindow {
		delete(s.attempts, email)
	}
	return 0
}

func (s *AuthService) recordFailure(email string) {
	s.attemptMu.Lock()
	defer s.attemptMu.Unlock()
	now := time.Now()
	rec := s.attempts[email]
	if rec == nil || now.Sub(rec.first) > loginWindow {
		rec = &loginAttempt{first: now}
		s.attempts[email] = rec
	}
	rec.failures++
	if rec.failures >= loginMaxFailures {
		rec.lockedTo = now.Add(loginLockout)
		rec.failures = 0
		rec.first = now
	}
}

func (s *AuthService) clearFailures(email string) {
	s.attemptMu.Lock()
	defer s.attemptMu.Unlock()
	delete(s.attempts, email)
}

type Claims struct {
	UserID string `json:"uid"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	// ImpersonatedBy is the admin who started this session. It is empty for a
	// normal login, and an access token that carries it is never paired with a
	// refresh token: ending impersonation means restoring the admin's own token.
	ImpersonatedBy string `json:"imp,omitempty"`
	jwt.RegisteredClaims
}

type AuthResult struct {
	AccessToken  string            `json:"access_token"`
	RefreshToken string            `json:"refresh_token"`
	ExpiresIn    int64             `json:"expires_in"`
	User         models.PublicUser `json:"user"`
}

func (s *AuthService) Login(ctx context.Context, email, password, userAgent, ip string) (*AuthResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return nil, BadRequest("email and password are required")
	}

	// Checked before touching the database so a locked account costs nothing.
	if wait := s.lockoutRemaining(email); wait > 0 {
		minutes := int(wait.Minutes()) + 1
		s.audit.Record(ctx, AuditEntry{
			ActorEmail: email, Action: "auth.login_locked", Entity: "user",
			Detail: fmt.Sprintf("blocked for %d more minute(s)", minutes), IP: ip,
		})
		return nil, NewError(429, "too_many_attempts",
			fmt.Sprintf("too many failed sign-in attempts. Try again in %d minute(s).", minutes))
	}

	user, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			s.recordFailure(email)
			return nil, Unauthorized("invalid email or password")
		}
		return nil, err
	}
	if !user.IsActive {
		return nil, Forbidden("account is disabled")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		s.recordFailure(email)
		s.audit.Record(ctx, AuditEntry{
			ActorEmail: email, Action: "auth.login_failed", Entity: "user", EntityID: user.ID, IP: ip,
		})
		return nil, Unauthorized("invalid email or password")
	}
	s.clearFailures(email)

	result, err := s.issue(ctx, user, userAgent, ip)
	if err != nil {
		return nil, err
	}
	s.audit.Record(ctx, AuditEntry{
		ActorID: user.ID, ActorEmail: user.Email, Action: "auth.login",
		Entity: "user", EntityID: user.ID, IP: ip,
	})
	return result, nil
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken, userAgent, ip string) (*AuthResult, error) {
	if refreshToken == "" {
		return nil, Unauthorized("refresh token is required")
	}
	hash := hashToken(refreshToken)
	stored, err := s.tokens.FindByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, Unauthorized("invalid refresh token")
		}
		return nil, err
	}
	if stored.RevokedAt != nil {
		// A revoked token being presented means it leaked or was replayed.
		// Kill the whole family and force a fresh login.
		_ = s.tokens.RevokeAllForUser(ctx, stored.UserID)
		return nil, Unauthorized("refresh token has been revoked, please sign in again")
	}
	if time.Now().UTC().After(stored.ExpiresAt) {
		return nil, Unauthorized("refresh token expired, please sign in again")
	}

	user, err := s.users.FindByID(ctx, stored.UserID)
	if err != nil {
		return nil, Unauthorized("user no longer exists")
	}
	if !user.IsActive {
		return nil, Forbidden("account is disabled")
	}

	result, err := s.issue(ctx, user, userAgent, ip)
	if err != nil {
		return nil, err
	}
	newHash := hashToken(result.RefreshToken)
	if err := s.tokens.Revoke(ctx, stored.ID, newHash); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *AuthService) Logout(ctx context.Context, refreshToken string) {
	if refreshToken == "" {
		return
	}
	if stored, err := s.tokens.FindByHash(ctx, hashToken(refreshToken)); err == nil {
		_ = s.tokens.Revoke(ctx, stored.ID, "")
	}
}

// ImpersonationTTL bounds a support session. It is deliberately short and there
// is no refresh token, so an impersonated session cannot outlive the window or
// be renewed silently — the admin's own token is what the browser goes back to.
const ImpersonationTTL = 60 * time.Minute

type ImpersonationResult struct {
	AccessToken  string            `json:"access_token"`
	ExpiresIn    int64             `json:"expires_in"`
	User         models.PublicUser `json:"user"`          // the target
	OriginalUser models.PublicUser `json:"original_user"` // the admin
}

// Impersonate issues an access token that acts as targetUserID.
//
// The token carries the target's role, so an admin viewing a normal user really
// does lose admin endpoints instead of only pretending to. The admin who started
// it is recorded on the token and in the audit log, because every action taken
// during the session will otherwise look like the target did it.
func (s *AuthService) Impersonate(ctx context.Context, caller Caller, targetUserID string, ip string) (*ImpersonationResult, error) {
	if !caller.Admin {
		return nil, Forbidden("admin role required")
	}
	targetUserID = strings.TrimSpace(targetUserID)
	if targetUserID == "" {
		return nil, Validation(map[string]string{"user_id": "required"})
	}
	if targetUserID == caller.ID {
		return nil, BadRequest("you cannot impersonate your own account")
	}

	admin, err := s.GetUser(ctx, caller.ID)
	if err != nil {
		return nil, err
	}
	target, err := s.GetUser(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	if !target.IsActive {
		return nil, BadRequest("target user account is inactive")
	}

	now := time.Now()
	claims := &Claims{
		UserID:         target.ID,
		Email:          target.Email,
		Role:           target.Role,
		ImpersonatedBy: admin.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   target.ID,
			ExpiresAt: jwt.NewNumericDate(now.Add(ImpersonationTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.cfg.JWTSecret)
	if err != nil {
		return nil, err
	}

	s.audit.Record(ctx, AuditEntry{
		ActorID: admin.ID, ActorEmail: admin.Email, Action: "user.impersonate_start",
		Entity: "user", EntityID: target.ID,
		Detail: fmt.Sprintf("%s (%s) now acts as %s", admin.Name, admin.Email, target.Email),
		IP:     ip,
	})

	return &ImpersonationResult{
		AccessToken:  signed,
		ExpiresIn:    int64(ImpersonationTTL.Seconds()),
		User:         target.Public(),
		OriginalUser: admin.Public(),
	}, nil
}

// EndImpersonation only records that the session ended. The browser restores the
// admin's own token; there is no refresh token to revoke.
func (s *AuthService) EndImpersonation(ctx context.Context, caller Caller, impersonatedBy string, ip string) {
	if impersonatedBy == "" {
		return
	}
	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "user.impersonate_end",
		Entity: "user", EntityID: caller.ID,
		Detail: "stopped acting as " + caller.Email,
		IP:     ip,
	})
}

func (s *AuthService) ValidateAccess(token string) (*Claims, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.cfg.JWTSecret, nil
	})
	if err != nil || !parsed.Valid {
		return nil, Unauthorized("invalid or expired access token")
	}
	return claims, nil
}

func (s *AuthService) GetUser(ctx context.Context, id string) (*models.User, error) {
	user, err := s.users.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, NotFound("user not found")
		}
		return nil, err
	}
	return user, nil
}

func (s *AuthService) ChangePassword(ctx context.Context, userID, current, next string) error {
	if len(next) < 8 {
		return Validation(map[string]string{"new_password": "must be at least 8 characters"})
	}
	user, err := s.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(current)); err != nil {
		return BadRequest("current password is incorrect")
	}
	hashed, err := HashPassword(next)
	if err != nil {
		return err
	}
	user.Password = hashed
	if err := s.users.Update(ctx, user); err != nil {
		return err
	}
	_ = s.tokens.RevokeAllForUser(ctx, user.ID)
	s.audit.Record(ctx, AuditEntry{
		ActorID: user.ID, ActorEmail: user.Email, Action: "auth.change_password",
		Entity: "user", EntityID: user.ID,
	})
	return nil
}

func (s *AuthService) issue(ctx context.Context, user *models.User, userAgent, ip string) (*AuthResult, error) {
	now := time.Now().UTC()
	claims := Claims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.cfg.AccessTTL)),
			Issuer:    "cf-multi-tunnel",
		},
	}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.cfg.JWTSecret)
	if err != nil {
		return nil, err
	}

	refreshRaw, err := randomToken(48)
	if err != nil {
		return nil, err
	}
	if err := s.tokens.Create(ctx, &models.RefreshToken{
		UserID:    user.ID,
		TokenHash: hashToken(refreshRaw),
		ExpiresAt: now.Add(s.cfg.RefreshTTL),
		UserAgent: truncate(userAgent, 255),
		IP:        ip,
	}); err != nil {
		return nil, err
	}

	return &AuthResult{
		AccessToken:  access,
		RefreshToken: refreshRaw,
		ExpiresIn:    int64(s.cfg.AccessTTL.Seconds()),
		User:         user.Public(),
	}, nil
}

func HashPassword(plain string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(hashed), err
}

// randomToken produces a URL-safe opaque token for refresh cookies/headers.
func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
