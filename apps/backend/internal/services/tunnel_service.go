package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"cf-multi-tunnel/backend/internal/cloudflare"
	"cf-multi-tunnel/backend/internal/config"
	"cf-multi-tunnel/backend/internal/models"
	db "cf-multi-tunnel/backend/internal/repositories/db"
)

type TunnelService struct {
	tunnels  *db.TunnelRepository
	accounts *db.AccountRepository
	zones    *db.ZoneRepository
	apps     *db.AppRepository
	cf       *CF
	audit    *AuditService
	conn     ConnectorController
	cfg      *config.Config

	// One mutex per tunnel: Cloudflare ingress is a whole-list PUT, so two
	// concurrent publishes on the same tunnel must not interleave.
	ingressLocks sync.Map
}

func NewTunnelService(
	tunnels *db.TunnelRepository,
	accounts *db.AccountRepository,
	zones *db.ZoneRepository,
	apps *db.AppRepository,
	cf *CF,
	audit *AuditService,
	conn ConnectorController,
	cfg *config.Config,
) *TunnelService {
	return &TunnelService{
		tunnels: tunnels, accounts: accounts, zones: zones, apps: apps,
		cf: cf, audit: audit, conn: conn, cfg: cfg,
	}
}

func (s *TunnelService) lockFor(tunnelID string) *sync.Mutex {
	actual, _ := s.ingressLocks.LoadOrStore(tunnelID, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

var tunnelNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)

func (s *TunnelService) List(ctx context.Context, caller Caller, accountID, location string) ([]models.Tunnel, error) {
	return s.tunnels.List(ctx, caller.Actor(), accountID, location)
}

func (s *TunnelService) Get(ctx context.Context, caller Caller, id string) (*models.Tunnel, error) {
	tunnel, err := s.tunnels.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, NotFound("tunnel not found")
		}
		return nil, err
	}
	if !caller.Admin && tunnel.OwnerID != caller.ID {
		return nil, Forbidden("you do not have access to this tunnel")
	}
	return tunnel, nil
}

type CreateTunnelInput struct {
	AccountID     string `json:"account_id"`
	Name          string `json:"name"`
	Location      string `json:"location"`
	HostLabel     string `json:"host_label"`
	ConnectorMode string `json:"connector_mode"`
}

// normalizeLocation defaults to "local" only when the caller says so. Callers
// that cannot know (sync/import) pass an explicit value.
func normalizeLocation(location string) (string, error) {
	switch location {
	case "":
		return "local", nil
	case "local", "remote":
		return location, nil
	default:
		return "", fmt.Errorf("location must be local or remote")
	}
}

func normalizeConnectorMode(mode, fallback string) string {
	if mode == "docker" || mode == "external" {
		return mode
	}
	return fallback
}

// Create makes a tunnel that is fully manageable from the panel: config_src is
// "cloudflare", so ingress is pushed through the API and no local config file
// is ever needed.
func (s *TunnelService) Create(ctx context.Context, caller Caller, in CreateTunnelInput) (*models.Tunnel, error) {
	name := strings.ToLower(strings.TrimSpace(in.Name))
	fields := map[string]string{}
	if in.AccountID == "" {
		fields["account_id"] = "required"
	}
	if !tunnelNameRe.MatchString(name) {
		fields["name"] = "2-63 chars, lowercase letters, digits and dashes"
	}
	location, err := normalizeLocation(in.Location)
	if err != nil {
		fields["location"] = err.Error()
	}
	if len(fields) > 0 {
		return nil, Validation(fields)
	}

	account, err := s.accounts.FindByID(ctx, in.AccountID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, NotFound("account not found")
		}
		return nil, err
	}
	if !caller.Admin && account.OwnerID != caller.ID {
		return nil, Forbidden("you do not have access to this account")
	}
	client, err := s.cf.ClientFor(account)
	if err != nil {
		return nil, err
	}

	remote, err := client.CreateTunnel(ctx, account.AccountID, name, "cloudflare")
	if err != nil {
		return nil, MapUpstream(err)
	}

	token, err := client.GetTunnelToken(ctx, account.AccountID, remote.ID)
	if err != nil {
		// The tunnel exists remotely; clean up so a retry does not hit a name clash.
		_ = client.DeleteTunnel(ctx, account.AccountID, remote.ID)
		return nil, MapUpstream(err)
	}
	encToken, err := s.cf.Encrypt(token)
	if err != nil {
		return nil, err
	}

	mode := normalizeConnectorMode(in.ConnectorMode, s.cfg.ConnectorMode)
	hostLabel := strings.TrimSpace(in.HostLabel)
	if location == "local" && hostLabel == "" {
		hostLabel = "this panel host"
	}

	tunnel := &models.Tunnel{
		OwnerID:        account.OwnerID,
		CFAccountID:    account.ID,
		TunnelID:       remote.ID,
		Name:           name,
		Credential:     "api",
		TunnelTokenEnc: encToken,
		ConfigSrc:      "cloudflare",
		Location:       location,
		HostLabel:      hostLabel,
		ConnectorMode:  mode,
		Status:         "stopped",
	}
	if err := s.tunnels.Create(ctx, tunnel); err != nil {
		return nil, err
	}

	// Write an empty ingress with the mandatory catch-all so the tunnel is
	// valid the moment it is created, not only after the first publish.
	if err := s.ReconcileIngress(ctx, tunnel); err != nil {
		tunnel.StatusMessage = "ingress not initialised: " + err.Error()
		_ = s.tunnels.Update(ctx, tunnel)
	}

	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "tunnel.create",
		Entity: "tunnel", EntityID: tunnel.ID, Detail: name + " / " + remote.ID,
	})
	return tunnel, nil
}
func (s *TunnelService) Delete(ctx context.Context, caller Caller, id string) error {
	tunnel, err := s.Get(ctx, caller, id)
	if err != nil {
		return err
	}

	// Deleting the tunnel cascades to its published_app rows. If one of them is
	// the panel exposure, that would silently unpublish the control plane and
	// leave its DNS record pointing at a tunnel that no longer exists. Make the
	// operator move or remove the exposure first.
	if apps, err := s.apps.ListByTunnel(ctx, tunnel.ID); err == nil {
		for _, app := range apps {
			if app.Kind == "panel" {
				return Conflict("this tunnel publishes the panel itself; move or remove the panel exposure in Settings before deleting it")
			}
		}
	}

	_ = s.conn.Stop(ctx, tunnel)
	_ = s.conn.Remove(ctx, tunnel)

	if account, err := s.accounts.FindByID(ctx, tunnel.CFAccountID); err == nil && account.HasAPI() {
		if client, err := s.cf.ClientFor(account); err == nil {
			if err := client.DeleteTunnel(ctx, account.AccountID, tunnel.TunnelID); err != nil && !cloudflare.IsNotFound(err) {
				return MapUpstream(err)
			}
		}
	}

	if err := s.tunnels.Delete(ctx, id); err != nil {
		return err
	}
	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "tunnel.delete",
		Entity: "tunnel", EntityID: id, Detail: tunnel.Name,
	})
	return nil
}

func (s *TunnelService) Start(ctx context.Context, caller Caller, id string) (*models.Tunnel, error) {
	tunnel, err := s.Get(ctx, caller, id)
	if err != nil {
		return nil, err
	}
	if tunnel.Location == "remote" {
		return nil, BadRequest(
			"this tunnel runs on " + orDefaultLabel(tunnel.HostLabel) + ". The panel cannot start a connector it does not host; " +
				"run cloudflared there with the token from the tunnel page.")
	}
	if tunnel.ConnectorMode != "docker" {
		return nil, BadRequest("this tunnel is in external mode; run cloudflared on your own host with the token from the tunnel page")
	}
	if !s.conn.Available() {
		return nil, NewError(503, "docker_unavailable", "Docker is not reachable from the panel, so the connector cannot be started")
	}

	if err := s.conn.EnsureRunning(ctx, tunnel); err != nil {
		tunnel.Status = "error"
		tunnel.StatusMessage = err.Error()
		_ = s.tunnels.Update(ctx, tunnel)
		return nil, Upstream(err)
	}

	now := time.Now().UTC()
	tunnel.DesiredRunning = true
	tunnel.Status = "running"
	tunnel.StatusMessage = ""
	tunnel.LastSeenAt = &now
	if err := s.tunnels.Update(ctx, tunnel); err != nil {
		return nil, err
	}
	s.conn.RecordRunningVersion(ctx, tunnel)
	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "tunnel.start",
		Entity: "tunnel", EntityID: tunnel.ID,
	})
	return tunnel, nil
}

func (s *TunnelService) Stop(ctx context.Context, caller Caller, id string) (*models.Tunnel, error) {
	tunnel, err := s.Get(ctx, caller, id)
	if err != nil {
		return nil, err
	}
	if !tunnel.Supervised() {
		return nil, BadRequest("this tunnel is not hosted by the panel, so there is nothing to stop here")
	}
	if err := s.conn.Stop(ctx, tunnel); err != nil {
		return nil, Upstream(err)
	}
	tunnel.DesiredRunning = false
	tunnel.Status = "stopped"
	tunnel.StatusMessage = ""
	if err := s.tunnels.Update(ctx, tunnel); err != nil {
		return nil, err
	}
	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "tunnel.stop",
		Entity: "tunnel", EntityID: tunnel.ID,
	})
	return tunnel, nil
}

// TokenFor returns a runnable cloudflared token. Prefers the stored one and
// fetches from the API only when it is missing.
func (s *TunnelService) TokenFor(ctx context.Context, t *models.Tunnel) (string, error) {
	if t.TunnelTokenEnc != "" {
		return s.cf.Decrypt(t.TunnelTokenEnc)
	}
	account, err := s.accounts.FindByID(ctx, t.CFAccountID)
	if err != nil {
		return "", err
	}
	client, err := s.cf.ClientFor(account)
	if err != nil {
		return "", err
	}
	token, err := client.GetTunnelToken(ctx, account.AccountID, t.TunnelID)
	if err != nil {
		return "", MapUpstream(err)
	}
	if enc, err := s.cf.Encrypt(token); err == nil {
		t.TunnelTokenEnc = enc
		_ = s.tunnels.Update(ctx, t)
	}
	return token, nil
}

// ReconcileIngress rebuilds the tunnel's entire ingress list from the panel's
// own published apps. Cloudflare has no partial update, so this is the only
// correct way to keep two editors from clobbering each other.
func (s *TunnelService) ReconcileIngress(ctx context.Context, tunnel *models.Tunnel) error {
	if !tunnel.RemoteConfigurable() {
		return nil
	}

	lock := s.lockFor(tunnel.ID)
	lock.Lock()
	defer lock.Unlock()

	account, err := s.accounts.FindByID(ctx, tunnel.CFAccountID)
	if err != nil {
		return err
	}
	client, err := s.cf.ClientFor(account)
	if err != nil {
		return err
	}

	apps, err := s.apps.ListEnabledByTunnel(ctx, tunnel.ID)
	if err != nil {
		return err
	}

	rules := make([]cloudflare.IngressRule, 0, len(apps)+1)
	for _, app := range apps {
		rule := cloudflare.IngressRule{
			Hostname: app.Hostname,
			Path:     app.Path,
			Service:  app.ServiceURL,
		}
		if app.OriginRequestJSON != "" {
			rule.OriginRequest = json.RawMessage(app.OriginRequestJSON)
		}
		rules = append(rules, rule)
	}
	// Mandatory final catch-all; Cloudflare rejects a config without it.
	rules = append(rules, cloudflare.IngressRule{Service: "http_status:404"})

	if err := client.PutTunnelConfig(ctx, account.AccountID, tunnel.TunnelID, cloudflare.TunnelConfig{Ingress: rules}); err != nil {
		return MapUpstream(err)
	}
	return nil
}

func (s *TunnelService) RefreshStatus(ctx context.Context, tunnel *models.Tunnel) {
	if tunnel.ManagedLocally() {
		if running, err := s.conn.IsRunning(ctx, tunnel); err == nil {
			if running {
				tunnel.Status = "running"
			} else {
				tunnel.Status = "stopped"
			}
		}
	}

	account, err := s.accounts.FindByID(ctx, tunnel.CFAccountID)
	if err != nil || !account.HasAPI() {
		_ = s.tunnels.Update(ctx, tunnel)
		return
	}
	client, err := s.cf.ClientFor(account)
	if err != nil {
		_ = s.tunnels.Update(ctx, tunnel)
		return
	}
	conns, err := client.GetConnections(ctx, account.AccountID, tunnel.TunnelID)
	if err != nil {
		_ = s.tunnels.Update(ctx, tunnel)
		return
	}

	if len(conns) > 0 {
		now := time.Now().UTC()
		tunnel.LastSeenAt = &now
		// For tunnels the panel does not host, Cloudflare is the only truth.
		// For local ones, Docker already answered and is more precise.
		if !tunnel.ManagedLocally() {
			tunnel.Status = "running"
		}
	} else if !tunnel.ManagedLocally() {
		tunnel.Status = "stopped"
	}
	_ = s.tunnels.Update(ctx, tunnel)
}

// RefreshAll is the periodic worker body.
func (s *TunnelService) RefreshAll(ctx context.Context) {
	tunnels, err := s.tunnels.List(ctx, db.Actor{Admin: true}, "", "")
	if err != nil {
		return
	}
	for i := range tunnels {
		s.RefreshStatus(ctx, &tunnels[i])
	}
}

// Logs returns the cloudflared container log for a docker-managed tunnel.
func (s *TunnelService) Logs(ctx context.Context, caller Caller, id string, lines int) (string, error) {
	tunnel, err := s.Get(ctx, caller, id)
	if err != nil {
		return "", err
	}
	if !tunnel.ManagedLocally() {
		if tunnel.Location == "remote" {
			return "", BadRequest("this tunnel runs on " + orDefaultLabel(tunnel.HostLabel) + "; open its logs on that device")
		}
		return "", BadRequest("this tunnel is in external mode; run cloudflared on your own host to see logs")
	}
	return s.conn.Logs(ctx, tunnel, lines)
}

// Update changes the placement metadata. Moving a tunnel from remote to local
// is allowed, but the panel will not silently start a container for it: the
// user still has to press Start.
func (s *TunnelService) Update(ctx context.Context, caller Caller, id string, in UpdateTunnelInput) (*models.Tunnel, error) {
	tunnel, err := s.Get(ctx, caller, id)
	if err != nil {
		return nil, err
	}

	fields := map[string]string{}
	// Resolve the target placement before touching the row so the availability
	// check sees the combination the user actually asked for.
	targetLocation := tunnel.Location
	if in.Location != nil {
		location, err := normalizeLocation(*in.Location)
		if err != nil {
			fields["location"] = err.Error()
		} else {
			targetLocation = location
		}
	}

	targetMode := tunnel.ConnectorMode
	if in.ConnectorMode != nil {
		if *in.ConnectorMode != "docker" && *in.ConnectorMode != "external" {
			fields["connector_mode"] = "must be docker or external"
		} else {
			targetMode = *in.ConnectorMode
		}
	}
	if targetLocation == "remote" {
		// Docker on a remote device is that device's business. Storing "docker"
		// here would imply the panel supervises it, which it cannot.
		targetMode = "external"
	}
	if targetLocation == "local" && targetMode == "docker" && !s.conn.Available() {
		fields["location"] = "Docker is not reachable from the panel, so this tunnel cannot be hosted here"
	}
	if len(fields) > 0 {
		return nil, Validation(fields)
	}

	wasManaged := tunnel.ManagedLocally()
	tunnel.Location = targetLocation
	tunnel.ConnectorMode = targetMode
	if in.HostLabel != nil {
		tunnel.HostLabel = strings.TrimSpace(*in.HostLabel)
	}
	if tunnel.Location == "local" && tunnel.HostLabel == "" {
		tunnel.HostLabel = "this panel host"
	}

	// Leaving local supervision must not leave an orphan container behind that
	// nothing will ever stop or update again.
	if wasManaged && !tunnel.ManagedLocally() {
		_ = s.conn.Stop(ctx, tunnel)
		_ = s.conn.Remove(ctx, tunnel)
		tunnel.DesiredRunning = false
		tunnel.Status = "unknown"
		tunnel.ImageVersion = ""
	}

	if err := s.tunnels.Update(ctx, tunnel); err != nil {
		return nil, err
	}
	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "tunnel.update",
		Entity: "tunnel", EntityID: tunnel.ID,
		Detail: tunnel.Location + " / " + tunnel.HostLabel + " / " + tunnel.ConnectorMode,
	})
	return tunnel, nil
}

type UpdateTunnelInput struct {
	Location      *string `json:"location"`
	HostLabel     *string `json:"host_label"`
	ConnectorMode *string `json:"connector_mode"`
}

func orDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func orDefaultLabel(label string) string {
	if strings.TrimSpace(label) == "" {
		return "another device"
	}
	return label
}

// ConfigYAML renders a locally-managed cloudflared config from the panel's
// published apps. Used for imported tunnels and for users who want to run the
// connector on their own machine.
func (s *TunnelService) ConfigYAML(ctx context.Context, caller Caller, id string) (string, error) {
	tunnel, err := s.Get(ctx, caller, id)
	if err != nil {
		return "", err
	}
	apps, err := s.apps.ListEnabledByTunnel(ctx, tunnel.ID)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("tunnel: " + tunnel.TunnelID + "\n")
	b.WriteString("credentials-file: /etc/cloudflared/creds.json\n")
	b.WriteString("no-autoupdate: true\n\n")
	b.WriteString("ingress:\n")
	for _, app := range apps {
		b.WriteString("  - hostname: " + app.Hostname + "\n")
		if app.Path != "" {
			b.WriteString("    path: " + app.Path + "\n")
		}
		b.WriteString("    service: " + app.ServiceURL + "\n")
		if app.OriginRequestJSON != "" {
			b.WriteString("    originRequest: " + app.OriginRequestJSON + "\n")
		}
	}
	b.WriteString("  - service: http_status:404\n")
	return b.String(), nil
}

// CredentialsJSON renders the file cloudflared expects for a locally-managed
// tunnel. The secret is recovered from the stored token, so API-created tunnels
// work here too.
func (s *TunnelService) CredentialsJSON(ctx context.Context, caller Caller, id string) (string, error) {
	tunnel, err := s.Get(ctx, caller, id)
	if err != nil {
		return "", err
	}
	account, err := s.accounts.FindByID(ctx, tunnel.CFAccountID)
	if err != nil {
		return "", err
	}
	token, err := s.TokenFor(ctx, tunnel)
	if err != nil {
		return "", err
	}
	payload, err := cloudflare.DecodeTunnelToken(token)
	if err != nil {
		return "", BadRequest("stored tunnel token cannot be decoded: " + err.Error())
	}
	out := map[string]string{
		"AccountTag":   account.AccountID,
		"TunnelSecret": payload.TunnelSecret,
		"TunnelID":     tunnel.TunnelID,
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
