package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"cf-multi-tunnel/backend/internal/config"
	"cf-multi-tunnel/backend/internal/models"
	db "cf-multi-tunnel/backend/internal/repositories/db"
)

type AppService struct {
	apps      *db.AppRepository
	tunnels   *db.TunnelRepository
	accounts  *db.AccountRepository
	zones     *db.ZoneRepository
	tunnelSvc *TunnelService
	cf        *CF
	audit     *AuditService
	cfg       *config.Config
}

func NewAppService(
	apps *db.AppRepository,
	tunnels *db.TunnelRepository,
	accounts *db.AccountRepository,
	zones *db.ZoneRepository,
	tunnelSvc *TunnelService,
	cf *CF,
	audit *AuditService,
	cfg *config.Config,
) *AppService {
	return &AppService{apps: apps, tunnels: tunnels, accounts: accounts, zones: zones, tunnelSvc: tunnelSvc, cf: cf, audit: audit, cfg: cfg}
}

type CreateAppInput struct {
	Hostname      string          `json:"hostname"`
	Subdomain     string          `json:"subdomain"`
	Domain        string          `json:"domain"`
	Path          string          `json:"path"`
	ServiceURL    string          `json:"service_url"`
	ManageDNS     *bool           `json:"manage_dns"`
	Proxied       *bool           `json:"proxied"`
	OriginRequest json.RawMessage `json:"origin_request"`
}

func (s *AppService) ListByTunnel(ctx context.Context, caller Caller, tunnelID string) ([]models.PublishedApp, error) {
	tunnel, err := s.tunnelSvc.Get(ctx, caller, tunnelID)
	if err != nil {
		return nil, err
	}
	return s.apps.ListByTunnel(ctx, tunnel.ID)
}

func (s *AppService) List(ctx context.Context, caller Caller) ([]models.PublishedApp, error) {
	return s.apps.List(ctx, caller.Actor())
}

func (s *AppService) Get(ctx context.Context, caller Caller, id string) (*models.PublishedApp, error) {
	app, err := s.apps.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, NotFound("published app not found")
		}
		return nil, err
	}
	if !caller.Admin && app.OwnerID != caller.ID {
		return nil, Forbidden("you do not have access to this app")
	}
	return app, nil
}

func (s *AppService) Create(ctx context.Context, caller Caller, tunnelID string, in CreateAppInput) (*models.PublishedApp, error) {
	tunnel, err := s.tunnelSvc.Get(ctx, caller, tunnelID)
	if err != nil {
		return nil, err
	}

	hostname, err := resolveHostname(in)
	if err != nil {
		return nil, err
	}
	serviceURL, err := ValidateServiceURL(in.ServiceURL)
	if err != nil {
		return nil, Validation(map[string]string{"service_url": err.Error()})
	}
	path, err := normalizePath(in.Path)
	if err != nil {
		return nil, Validation(map[string]string{"path": err.Error()})
	}

	if existing, err := s.apps.FindByHostname(ctx, hostname); err == nil && existing != nil {
		return nil, Conflict("hostname " + hostname + " is already published by another app")
	} else if err != nil && !errors.Is(err, db.ErrNotFound) {
		return nil, err
	}

	manageDNS := true
	if in.ManageDNS != nil {
		manageDNS = *in.ManageDNS
	}
	proxied := true
	if in.Proxied != nil {
		proxied = *in.Proxied
	}

	account, err := s.accounts.FindByID(ctx, tunnel.CFAccountID)
	if err != nil {
		return nil, err
	}
	zone, zoneErr := s.resolveZone(ctx, account.ID, hostname)
	if manageDNS && zoneErr != nil {
		return nil, zoneErr
	}
	if !tunnel.RemoteConfigurable() {
		// No API access means no DNS automation either; make that explicit and
		// fall back to the downloadable config.yml path.
		manageDNS = false
	}

	app := &models.PublishedApp{
		OwnerID:    tunnel.OwnerID,
		TunnelID:   tunnel.ID,
		Kind:       "app",
		Hostname:   hostname,
		Path:       path,
		ServiceURL: serviceURL,
		ManageDNS:  manageDNS,
		Proxied:    proxied,
		Enabled:    true,
		SyncStatus: "pending",
	}
	if zone != nil {
		app.ZoneID = zone.ZoneID
		app.ZoneName = zone.Name
	}
	if len(in.OriginRequest) > 0 && string(in.OriginRequest) != "null" {
		if !json.Valid(in.OriginRequest) {
			return nil, Validation(map[string]string{"origin_request": "must be valid JSON"})
		}
		app.OriginRequestJSON = string(in.OriginRequest)
	}

	if err := s.apps.Create(ctx, app); err != nil {
		return nil, err
	}

	s.apply(ctx, app, tunnel, account)
	_ = s.apps.Update(ctx, app)

	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "app.create",
		Entity: "published_app", EntityID: app.ID, Detail: app.Hostname + " -> " + app.ServiceURL,
	})
	return app, nil
}

func (s *AppService) Update(ctx context.Context, caller Caller, id string, in CreateAppInput) (*models.PublishedApp, error) {
	app, err := s.Get(ctx, caller, id)
	if err != nil {
		return nil, err
	}
	tunnel, err := s.tunnels.FindByID(ctx, app.TunnelID)
	if err != nil {
		return nil, err
	}
	account, err := s.accounts.FindByID(ctx, tunnel.CFAccountID)
	if err != nil {
		return nil, err
	}

	hostname, err := resolveHostname(in)
	if err != nil {
		return nil, err
	}
	serviceURL, err := ValidateServiceURL(in.ServiceURL)
	if err != nil {
		return nil, Validation(map[string]string{"service_url": err.Error()})
	}
	path, err := normalizePath(in.Path)
	if err != nil {
		return nil, Validation(map[string]string{"path": err.Error()})
	}

	if hostname != app.Hostname {
		if existing, err := s.apps.FindByHostname(ctx, hostname); err == nil && existing != nil && existing.ID != app.ID {
			return nil, Conflict("hostname " + hostname + " is already published by another app")
		}
		// Remove the old DNS record before claiming the new name.
		if app.ManageDNS && app.ZoneID != "" && app.DNSRecordID != "" {
			if client, err := s.cf.ClientFor(account); err == nil {
				_ = client.DeleteDNSRecord(ctx, app.ZoneID, app.DNSRecordID)
			}
		}
		app.DNSRecordID = ""
	}

	app.Hostname = hostname
	app.Path = path
	app.ServiceURL = serviceURL
	if in.ManageDNS != nil {
		app.ManageDNS = *in.ManageDNS && tunnel.RemoteConfigurable()
	}
	if in.Proxied != nil {
		app.Proxied = *in.Proxied
	}
	if len(in.OriginRequest) > 0 && string(in.OriginRequest) != "null" {
		if !json.Valid(in.OriginRequest) {
			return nil, Validation(map[string]string{"origin_request": "must be valid JSON"})
		}
		app.OriginRequestJSON = string(in.OriginRequest)
	}
	if zone, zoneErr := s.resolveZone(ctx, account.ID, hostname); zoneErr == nil && zone != nil {
		app.ZoneID = zone.ZoneID
		app.ZoneName = zone.Name
	} else if app.ManageDNS {
		return nil, zoneErr
	}

	if err := s.apps.Update(ctx, app); err != nil {
		return nil, err
	}
	s.apply(ctx, app, tunnel, account)
	_ = s.apps.Update(ctx, app)

	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "app.update",
		Entity: "published_app", EntityID: app.ID, Detail: app.Hostname,
	})
	return app, nil
}

func (s *AppService) Delete(ctx context.Context, caller Caller, id string) error {
	app, err := s.Get(ctx, caller, id)
	if err != nil {
		return err
	}
	if app.Kind == "panel" {
		return Forbidden("the panel exposure is managed from Settings, not from the app list")
	}
	tunnel, err := s.tunnels.FindByID(ctx, app.TunnelID)
	if err != nil {
		return err
	}

	if app.ManageDNS && app.ZoneID != "" && app.DNSRecordID != "" {
		if account, err := s.accounts.FindByID(ctx, tunnel.CFAccountID); err == nil {
			if client, err := s.cf.ClientFor(account); err == nil {
				if err := client.DeleteDNSRecord(ctx, app.ZoneID, app.DNSRecordID); err != nil {
					return MapUpstream(err)
				}
			}
		}
	}

	if err := s.apps.Delete(ctx, id); err != nil {
		return err
	}
	// DB is now the source of truth; push the new list.
	_ = s.tunnelSvc.ReconcileIngress(ctx, tunnel)

	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "app.delete",
		Entity: "published_app", EntityID: id, Detail: app.Hostname,
	})
	return nil
}

func (s *AppService) SetEnabled(ctx context.Context, caller Caller, id string, enabled bool) (*models.PublishedApp, error) {
	app, err := s.Get(ctx, caller, id)
	if err != nil {
		return nil, err
	}
	tunnel, err := s.tunnels.FindByID(ctx, app.TunnelID)
	if err != nil {
		return nil, err
	}
	app.Enabled = enabled
	if err := s.apps.Update(ctx, app); err != nil {
		return nil, err
	}
	_ = s.tunnelSvc.ReconcileIngress(ctx, tunnel)
	return app, nil
}

// Retry re-runs ingress + DNS for one app, for when the token permissions were
// fixed after the first attempt.
func (s *AppService) Retry(ctx context.Context, caller Caller, id string) (*models.PublishedApp, error) {
	app, err := s.Get(ctx, caller, id)
	if err != nil {
		return nil, err
	}
	tunnel, err := s.tunnels.FindByID(ctx, app.TunnelID)
	if err != nil {
		return nil, err
	}
	account, err := s.accounts.FindByID(ctx, tunnel.CFAccountID)
	if err != nil {
		return nil, err
	}
	s.apply(ctx, app, tunnel, account)
	_ = s.apps.Update(ctx, app)
	return app, nil
}

// apply drives one app toward its desired state. It never returns an error to
// the caller: a partial Cloudflare failure must not lose the row. The result is
// recorded in sync_status / last_error instead.
func (s *AppService) apply(ctx context.Context, app *models.PublishedApp, tunnel *models.Tunnel, account *models.CFAccount) {
	if !tunnel.RemoteConfigurable() {
		app.SyncStatus = "manual"
		app.LastError = fmt.Sprintf(
			"this tunnel is not remotely configurable (config_src=%s). Download config.yml and create this DNS record manually: %s CNAME %s.cfargotunnel.com",
			orDefault(tunnel.ConfigSrc, "unknown"), app.Hostname, tunnel.TunnelID,
		)
		return
	}

	if err := s.tunnelSvc.ReconcileIngress(ctx, tunnel); err != nil {
		app.SyncStatus = "error"
		app.LastError = "ingress: " + err.Error()
		return
	}

	if app.ManageDNS && app.ZoneID != "" {
		client, err := s.cf.ClientFor(account)
		if err != nil {
			app.SyncStatus = "error"
			app.LastError = err.Error()
			return
		}
		recordID, err := client.EnsureDNSRecord(ctx, app.ZoneID, app.Hostname, tunnel.TunnelID+".cfargotunnel.com", app.Proxied)
		if err != nil {
			app.SyncStatus = "error"
			app.LastError = "dns: " + err.Error()
			return
		}
		app.DNSRecordID = recordID
	} else if app.ManageDNS {
		app.SyncStatus = "error"
		app.LastError = "no Cloudflare zone matches " + app.Hostname + "; sync zones or disable DNS management"
		return
	}

	app.SyncStatus = "synced"
	app.LastError = ""
}

func (s *AppService) resolveZone(ctx context.Context, accountID, hostname string) (*models.CFZone, error) {
	zones, err := s.zones.ListByAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	zone := MatchZone(zones, hostname)
	if zone == nil {
		return nil, Validation(map[string]string{
			"hostname": "no cached Cloudflare zone matches " + hostname + "; sync the account zones first, or disable DNS management",
		})
	}
	return zone, nil
}

// ── Panel exposure ───────────────────────────────────────────────────

type PanelExposureInput struct {
	TunnelID  string `json:"tunnel_id"`
	Hostname  string `json:"hostname"`
	Subdomain string `json:"subdomain"`
	Domain    string `json:"domain"`
	// ServiceURL is only used when the tunnel runs on another device. For a
	// local tunnel the panel's own Docker-network address is used.
	ServiceURL string `json:"service_url"`
}

// checkNoLoop refuses a service URL that resolves back to the hostname being
// published. Without this, a remote connector would proxy the panel to itself
// and the site would hang instead of returning a clear error.
func checkNoLoop(serviceURL string, in PanelExposureInput, publicURL string) error {
	u, err := url.Parse(serviceURL)
	if err != nil || u.Host == "" {
		return Validation(map[string]string{"service_url": "must be an absolute URL such as http://192.168.1.10:7777"})
	}
	target := u.Hostname()
	hostname := strings.TrimSpace(in.Hostname)
	if hostname == "" && in.Subdomain != "" && in.Domain != "" {
		hostname = in.Subdomain + "." + in.Domain
	}
	if hostname != "" && strings.EqualFold(target, hostname) {
		return Validation(map[string]string{
			"service_url": "points at the hostname being published; use the panel's own reachable address instead",
		})
	}
	if pub, err := url.Parse(publicURL); err == nil && pub.Hostname() != "" && strings.EqualFold(target, pub.Hostname()) {
		return Validation(map[string]string{
			"service_url": "points at PUBLIC_URL, which is this same panel; a remote connector cannot reach it that way",
		})
	}
	return nil
}

func (s *AppService) GetPanelExposure(ctx context.Context) (*models.PublishedApp, error) {
	apps, err := s.apps.List(ctx, db.Actor{Admin: true})
	if err != nil {
		return nil, err
	}
	for i := range apps {
		if apps[i].Kind == "panel" {
			return &apps[i], nil
		}
	}
	return nil, nil
}

// SetPanelExposure publishes this dashboard through one of the managed tunnels.
// Admin-only: it exposes the control plane itself, so a normal user must never
// be able to redirect it.
func (s *AppService) SetPanelExposure(ctx context.Context, caller Caller, in PanelExposureInput) (*models.PublishedApp, error) {
	if !caller.Admin {
		return nil, Forbidden("only an admin can publish the panel")
	}
	if in.TunnelID == "" {
		return nil, Validation(map[string]string{"tunnel_id": "choose a tunnel"})
	}
	tunnel, err := s.tunnelSvc.Get(ctx, caller, in.TunnelID)
	if err != nil {
		return nil, err
	}
	if !tunnel.RemoteConfigurable() {
		return nil, BadRequest("the panel can only be published through a tunnel with an API token (config_src=cloudflare)")
	}
	// The service URL depends on where the connector runs. A local container
	// reaches the panel over the Docker network. A connector on another device
	// can only reach it over the network — and must NOT be handed the public
	// hostname being created here, or every request would loop back into the
	// tunnel. So remote exposure requires an explicit, loopback-free URL.
	var serviceURL string
	if tunnel.Location == "remote" {
		serviceURL = strings.TrimSpace(in.ServiceURL)
		if serviceURL == "" {
			return nil, Validation(map[string]string{
				"service_url": "required for a tunnel on another device, e.g. http://192.168.1.10:7777 " +
					"(it must not be the hostname you are publishing)",
			})
		}
		if err := checkNoLoop(serviceURL, in, s.cfg.PublicURL); err != nil {
			return nil, err
		}
	} else {
		serviceURL = s.cfg.PanelInternalURL
	}
	hostname, err := resolveHostname(CreateAppInput{Hostname: in.Hostname, Subdomain: in.Subdomain, Domain: in.Domain})
	if err != nil {
		return nil, err
	}
	account, err := s.accounts.FindByID(ctx, tunnel.CFAccountID)
	if err != nil {
		return nil, err
	}
	zone, err := s.resolveZone(ctx, account.ID, hostname)
	if err != nil {
		return nil, err
	}

	existing, _ := s.GetPanelExposure(ctx)
	previousTunnelID := ""
	if existing != nil {
		previousTunnelID = existing.TunnelID
		if existing.Hostname != hostname && existing.ZoneID != "" && existing.DNSRecordID != "" {
			if client, err := s.cf.ClientFor(account); err == nil {
				_ = client.DeleteDNSRecord(ctx, existing.ZoneID, existing.DNSRecordID)
			}
		}
		existing.TunnelID = tunnel.ID
		existing.OwnerID = caller.ID
		existing.Hostname = hostname
		existing.Path = "/*"
		existing.ServiceURL = serviceURL
		existing.ZoneID = zone.ZoneID
		existing.ZoneName = zone.Name
		existing.ManageDNS = true
		existing.Proxied = true
		existing.Enabled = true
		existing.SyncStatus = "pending"
		existing.DNSRecordID = ""
		if err := s.apps.Update(ctx, existing); err != nil {
			return nil, err
		}
		s.apply(ctx, existing, tunnel, account)
		_ = s.apps.Update(ctx, existing)
		if previousTunnelID != "" && previousTunnelID != tunnel.ID {
			if oldTunnel, err := s.tunnels.FindByID(ctx, previousTunnelID); err == nil {
				_ = s.tunnelSvc.ReconcileIngress(ctx, oldTunnel)
			}
		}
		s.audit.Record(ctx, AuditEntry{
			ActorID: caller.ID, ActorEmail: caller.Email, Action: "panel.expose",
			Entity: "published_app", EntityID: existing.ID, Detail: hostname,
		})
		return existing, nil
	}

	app := &models.PublishedApp{
		OwnerID:    caller.ID,
		TunnelID:   tunnel.ID,
		Kind:       "panel",
		Hostname:   hostname,
		Path:       "/*",
		ServiceURL: serviceURL,
		ZoneID:     zone.ZoneID,
		ZoneName:   zone.Name,
		ManageDNS:  true,
		Proxied:    true,
		Enabled:    true,
		SyncStatus: "pending",
	}
	if err := s.apps.Create(ctx, app); err != nil {
		return nil, err
	}
	s.apply(ctx, app, tunnel, account)
	_ = s.apps.Update(ctx, app)

	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "panel.expose",
		Entity: "published_app", EntityID: app.ID, Detail: hostname,
	})
	return app, nil
}

func (s *AppService) RemovePanelExposure(ctx context.Context, caller Caller) error {
	if !caller.Admin {
		return Forbidden("only an admin can change the panel exposure")
	}
	app, _ := s.GetPanelExposure(ctx)
	if app == nil {
		return nil
	}
	tunnel, err := s.tunnels.FindByID(ctx, app.TunnelID)
	if err != nil {
		return err
	}
	if app.ZoneID != "" && app.DNSRecordID != "" {
		if account, err := s.accounts.FindByID(ctx, tunnel.CFAccountID); err == nil {
			if client, err := s.cf.ClientFor(account); err == nil {
				_ = client.DeleteDNSRecord(ctx, app.ZoneID, app.DNSRecordID)
			}
		}
	}
	if err := s.apps.Delete(ctx, app.ID); err != nil {
		return err
	}
	_ = s.tunnelSvc.ReconcileIngress(ctx, tunnel)
	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "panel.unexpose",
		Entity: "published_app", EntityID: app.ID,
	})
	return nil
}

func resolveHostname(in CreateAppInput) (string, error) {
	raw := strings.TrimSpace(in.Hostname)
	if raw == "" && in.Subdomain != "" && in.Domain != "" {
		raw = in.Subdomain + "." + in.Domain
	}
	hostname, err := NormalizeHostname(raw)
	if err != nil {
		return "", Validation(map[string]string{"hostname": err.Error()})
	}
	return hostname, nil
}

func normalizePath(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "/*", nil
	}
	if !strings.HasPrefix(p, "/") {
		return "", errors.New("must start with /")
	}
	return p, nil
}
