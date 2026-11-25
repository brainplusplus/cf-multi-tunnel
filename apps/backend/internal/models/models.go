package models

import (
	"time"

	"cf-multi-tunnel/backend/internal/domain/identity"

	"gorm.io/gorm"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

type User struct {
	ID        string    `json:"id" gorm:"primaryKey;type:text"`
	Email     string    `json:"email" gorm:"uniqueIndex;not null"`
	Password  string    `json:"-" gorm:"not null"`
	Name      string    `json:"name" gorm:"not null"`
	Role      string    `json:"role" gorm:"not null;default:user"`
	IsActive  bool      `json:"is_active" gorm:"default:true"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (u *User) BeforeCreate(*gorm.DB) error {
	if u.ID == "" {
		u.ID = identity.New()
	}
	return nil
}

func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }

// PublicUser is what the API is allowed to return.
type PublicUser struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

func (u *User) Public() PublicUser {
	return PublicUser{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		Role:      u.Role,
		IsActive:  u.IsActive,
		CreatedAt: u.CreatedAt,
	}
}

// RefreshToken stores only a SHA-256 hash. Rotation marks the old row revoked
// and links it to its replacement so replay can be detected.
type RefreshToken struct {
	ID         string     `json:"id" gorm:"primaryKey;type:text"`
	UserID     string     `json:"user_id" gorm:"index;not null"`
	TokenHash  string     `json:"-" gorm:"uniqueIndex;not null"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	ReplacedBy string     `json:"replaced_by"`
	UserAgent  string     `json:"user_agent"`
	IP         string     `json:"ip"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (r *RefreshToken) BeforeCreate(*gorm.DB) error {
	if r.ID == "" {
		r.ID = identity.New()
	}
	return nil
}

// CFAccount is one Cloudflare account owned by a panel user. It holds either a
// scoped API token (full management) or only an imported tunnel token
// (credential_type = token_import, no account-level API access).
type CFAccount struct {
	ID            string     `json:"id" gorm:"primaryKey;type:text"`
	OwnerID       string     `json:"owner_id" gorm:"index;not null"`
	Name          string     `json:"name" gorm:"not null"`
	AccountID     string     `json:"account_id" gorm:"index;not null"`
	AccountName   string     `json:"account_name"`
	Credential    string     `json:"credential" gorm:"not null;default:api"` // api | token_import
	APITokenEnc   string     `json:"-" gorm:"column:api_token_enc"`
	TokenHint     string     `json:"token_hint"`
	Status        string     `json:"status" gorm:"default:active"` // active | invalid | error
	StatusMessage string     `json:"status_message"`
	LastCheckedAt *time.Time `json:"last_checked_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (a *CFAccount) BeforeCreate(*gorm.DB) error {
	if a.ID == "" {
		a.ID = identity.New()
	}
	return nil
}

func (a *CFAccount) HasAPI() bool { return a.Credential == "api" && a.APITokenEnc != "" }

// CFZone is a cached zone belonging to an account. Cached so hostname
// validation does not hit the Cloudflare API on every keystroke.
type CFZone struct {
	ID          string    `json:"id" gorm:"primaryKey;type:text"`
	CFAccountID string    `json:"cf_account_id" gorm:"index;not null"`
	ZoneID      string    `json:"zone_id" gorm:"not null"`
	Name        string    `json:"name" gorm:"index;not null"`
	Status      string    `json:"status"`
	SyncedAt    time.Time `json:"synced_at"`
}

func (z *CFZone) BeforeCreate(*gorm.DB) error {
	if z.ID == "" {
		z.ID = identity.New()
	}
	return nil
}

// Tunnel mirrors one Cloudflare tunnel.
//
// Two independent facts decide what the panel may do with it:
//
//	Location  = where cloudflared runs. "local" means this panel's own host,
//	            "remote" means some other device the panel cannot reach.
//	ConnectorMode = how it runs on that host ("docker" or "external").
//
// Only Location=local + ConnectorMode=docker is actually supervised. Everything
// else is observed through the Cloudflare API instead of guessed at.
type Tunnel struct {
	ID              string     `json:"id" gorm:"primaryKey;type:text"`
	OwnerID         string     `json:"owner_id" gorm:"index;not null"`
	CFAccountID     string     `json:"cf_account_id" gorm:"index;not null"`
	TunnelID        string     `json:"tunnel_id" gorm:"index;not null"`
	Name            string     `json:"name" gorm:"not null"`
	Credential      string     `json:"credential" gorm:"not null;default:api"` // api | token_import
	TunnelSecretEnc string     `json:"-" gorm:"column:tunnel_secret_enc"`
	TunnelTokenEnc  string     `json:"-" gorm:"column:tunnel_token_enc"`
	ConfigSrc       string     `json:"config_src" gorm:"default:cloudflare"` // cloudflare | local | unknown
	Location        string     `json:"location" gorm:"default:local;index"`  // local | remote
	HostLabel       string     `json:"host_label"`                           // "this server", "vps-jakarta", ...
	ConnectorMode   string     `json:"connector_mode" gorm:"default:docker"` // docker | external
	DesiredRunning  bool       `json:"desired_running" gorm:"default:false"`
	Status          string     `json:"status" gorm:"default:unknown"` // running | stopped | unknown | error
	StatusMessage   string     `json:"status_message"`
	ImageRef        string     `json:"image_ref"`
	ImageVersion    string     `json:"image_version"`
	LastSeenAt      *time.Time `json:"last_seen_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// ManagedLocally is the single gate for container operations. Any code that
// would touch Docker must ask this first, so a tunnel living on someone else's
// laptop can never be started, stopped or inspected here.
func (t *Tunnel) ManagedLocally() bool {
	return t.Location == "local" && t.ConnectorMode == "docker"
}

// Supervised is true when the panel owns the connector lifecycle at all.
func (t *Tunnel) Supervised() bool {
	return t.Location == "local"
}

func (t *Tunnel) BeforeCreate(*gorm.DB) error {
	if t.ID == "" {
		t.ID = identity.New()
	}
	return nil
}

func (t *Tunnel) RemoteConfigurable() bool { return t.ConfigSrc == "cloudflare" }

// PublishedApp maps one hostname to one origin service. Hostname is globally
// unique: Cloudflare refuses two tunnels claiming the same hostname, so the
// panel rejects it before the API does.
type PublishedApp struct {
	ID                string    `json:"id" gorm:"primaryKey;type:text"`
	OwnerID           string    `json:"owner_id" gorm:"index;not null"`
	TunnelID          string    `json:"tunnel_id" gorm:"index;not null"`
	Kind              string    `json:"kind" gorm:"default:app"` // app | panel
	Hostname          string    `json:"hostname" gorm:"uniqueIndex;not null"`
	Path              string    `json:"path" gorm:"default:/*"`
	ServiceURL        string    `json:"service_url" gorm:"not null"`
	ZoneID            string    `json:"zone_id"`
	ZoneName          string    `json:"zone_name"`
	DNSRecordID       string    `json:"dns_record_id"`
	ManageDNS         bool      `json:"manage_dns" gorm:"default:true"`
	Proxied           bool      `json:"proxied" gorm:"default:true"`
	OriginRequestJSON string    `json:"-" gorm:"column:origin_request_json"`
	Enabled           bool      `json:"enabled" gorm:"default:true"`
	SyncStatus        string    `json:"sync_status" gorm:"default:pending"` // synced | pending | error
	LastError         string    `json:"last_error"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (p *PublishedApp) BeforeCreate(*gorm.DB) error {
	if p.ID == "" {
		p.ID = identity.New()
	}
	return nil
}

type AuditLog struct {
	ID         string    `json:"id" gorm:"primaryKey;type:text"`
	ActorID    string    `json:"actor_id" gorm:"index"`
	ActorEmail string    `json:"actor_email"`
	Action     string    `json:"action" gorm:"index"`
	Entity     string    `json:"entity" gorm:"index"`
	EntityID   string    `json:"entity_id"`
	Detail     string    `json:"detail"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"created_at" gorm:"index"`
}

func (l *AuditLog) BeforeCreate(*gorm.DB) error {
	if l.ID == "" {
		l.ID = identity.New()
	}
	return nil
}

// Setting is a tiny key/value store for runtime-adjustable configuration that
// must survive a restart without editing .env (currently: the connector image).
type Setting struct {
	Key       string    `json:"key" gorm:"primaryKey;type:text"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}
