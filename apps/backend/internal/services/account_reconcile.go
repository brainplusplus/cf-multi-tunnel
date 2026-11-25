package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cf-multi-tunnel/backend/internal/cloudflare"
	"cf-multi-tunnel/backend/internal/models"
	db "cf-multi-tunnel/backend/internal/repositories/db"
)

// ReconcileResult reports what one account reconcile changed, so the UI can
// tell the user their existing tunnels and hostnames came along instead of
// leaving them to discover that by hand.
type ReconcileResult struct {
	Tunnels        int      `json:"tunnels"`
	TunnelsCreated int      `json:"tunnels_created"`
	TunnelsUpdated int      `json:"tunnels_updated"`
	TunnelsMissing int      `json:"tunnels_missing"`
	Apps           int      `json:"apps"`
	AppsCreated    int      `json:"apps_created"`
	AppsUpdated    int      `json:"apps_updated"`
	Skipped        []string `json:"skipped,omitempty"`
}

// ReconcileAccount makes the panel match what Cloudflare already holds for this
// account. It is an upsert in the two directions that matter:
//
//   - a tunnel that exists remotely but not locally is created,
//   - a tunnel that exists in both is refreshed (name, config source, and the
//     credential that now backs it) while its placement — the location and
//     host label the user chose — is left untouched,
//   - hostnames found in a tunnel's remote ingress become published apps, so a
//     later publish rebuilds the same list instead of wiping routes the panel
//     never knew about.
//
// It never deletes. A tunnel or hostname that vanished remotely is counted, not
// removed: a token that is missing one permission must not destroy working
// configuration.
func (s *TunnelService) ReconcileAccount(ctx context.Context, caller Caller, accountID string) (*ReconcileResult, error) {
	account, err := s.accounts.FindByID(ctx, accountID)
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

	remote, err := client.ListTunnels(ctx, account.AccountID)
	if err != nil {
		return nil, MapUpstream(err)
	}

	existing, err := s.tunnels.ListByAccount(ctx, account.ID)
	if err != nil {
		return nil, err
	}
	byRemote := make(map[string]*models.Tunnel, len(existing))
	for i := range existing {
		byRemote[existing[i].TunnelID] = &existing[i]
	}

	zones, err := s.zones.ListByAccount(ctx, account.ID)
	if err != nil {
		return nil, err
	}

	res := &ReconcileResult{Tunnels: len(remote)}
	seen := make(map[string]bool, len(remote))

	for _, rt := range remote {
		seen[rt.ID] = true

		local, known := byRemote[rt.ID]
		if !known {
			local = &models.Tunnel{
				OwnerID:       account.OwnerID,
				CFAccountID:   account.ID,
				TunnelID:      rt.ID,
				Name:          rt.Name,
				Credential:    "api",
				ConfigSrc:     orDefault(rt.ConfigSrc, "cloudflare"),
				Location:      "remote",
				HostLabel:     "discovered on Cloudflare",
				ConnectorMode: "external",
				Status:        "unknown",
			}
			if err := s.tunnels.Create(ctx, local); err != nil {
				return nil, err
			}
			res.TunnelsCreated++
		} else {
			changed := false
			if rt.Name != "" && local.Name != rt.Name {
				local.Name, changed = rt.Name, true
			}
			if rt.ConfigSrc != "" && local.ConfigSrc != rt.ConfigSrc {
				local.ConfigSrc, changed = rt.ConfigSrc, true
			}
			// A row imported by tunnel token has no usable API credential.
			// Reaching it through the API proves it does now.
			if local.Credential != "api" {
				local.Credential, changed = "api", true
			}
			if changed {
				if err := s.tunnels.Update(ctx, local); err != nil {
					return nil, err
				}
				res.TunnelsUpdated++
			}
		}

		if !local.RemoteConfigurable() {
			continue
		}
		if err := s.importIngress(ctx, account, client, local, zones, res); err != nil {
			res.Skipped = append(res.Skipped, local.Name+": "+err.Error())
		}
	}

	for i := range existing {
		if !seen[existing[i].TunnelID] {
			res.TunnelsMissing++
		}
	}

	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "account.reconcile",
		Entity: "cf_account", EntityID: account.ID,
		Detail: fmt.Sprintf("%d tunnels (%d new), %d apps (%d new)",
			res.Tunnels, res.TunnelsCreated, res.Apps, res.AppsCreated),
	})
	return res, nil
}

// importIngress turns the tunnel's remote ingress rules into published apps.
//
// Rules without a hostname (the http_status catch-all) carry no hostname and are
// skipped. A hostname already owned by another tunnel is reported rather than
// stolen: two tunnels cannot legitimately serve the same name.
func (s *TunnelService) importIngress(
	ctx context.Context,
	account *models.CFAccount,
	client *cloudflare.Client,
	tunnel *models.Tunnel,
	zones []models.CFZone,
	res *ReconcileResult,
) error {
	cfg, err := client.GetTunnelConfig(ctx, account.AccountID, tunnel.TunnelID)
	if err != nil {
		return MapUpstream(err)
	}

	owned, err := s.apps.ListByTunnel(ctx, tunnel.ID)
	if err != nil {
		return err
	}
	byHost := make(map[string]*models.PublishedApp, len(owned))
	for i := range owned {
		byHost[owned[i].Hostname] = &owned[i]
	}

	for _, rule := range cfg.Ingress {
		host := strings.ToLower(strings.TrimSpace(rule.Hostname))
		service := strings.TrimSpace(rule.Service)
		if host == "" || strings.HasPrefix(service, "http_status:") {
			continue
		}
		res.Apps++

		if app, ok := byHost[host]; ok {
			changed := false
			if service != "" && app.ServiceURL != service {
				app.ServiceURL, changed = service, true
			}
			if app.SyncStatus != "synced" {
				app.SyncStatus, app.LastError, changed = "synced", "", true
			}
			if changed {
				if err := s.apps.Update(ctx, app); err != nil {
					res.Skipped = append(res.Skipped, host+": "+err.Error())
					continue
				}
				res.AppsUpdated++
			}
			continue
		}

		// Hostname already published on a different tunnel or as the panel.
		if other, err := s.apps.FindByHostname(ctx, host); err == nil && other != nil {
			res.Skipped = append(res.Skipped, host+" is already published on another tunnel")
			continue
		}

		app := &models.PublishedApp{
			OwnerID:    account.OwnerID,
			TunnelID:   tunnel.ID,
			Kind:       "app",
			Hostname:   host,
			Path:       orDefault(rule.Path, "/*"),
			ServiceURL: service,
			Proxied:    true,
			Enabled:    true,
			// The route already exists in Cloudflare; it is synced by
			// definition. DNS is adopted only when a matching record is found.
			SyncStatus: "synced",
		}
		if len(rule.OriginRequest) > 0 && string(rule.OriginRequest) != "null" {
			app.OriginRequestJSON = string(rule.OriginRequest)
		}
		if zone := MatchZone(zones, host); zone != nil {
			app.ZoneID, app.ZoneName = zone.ZoneID, zone.Name
			if recs, err := client.ListDNSRecords(ctx, zone.ZoneID, host); err == nil {
				target := tunnel.TunnelID + ".cfargotunnel.com"
				for _, rec := range recs {
					if rec.Type == "CNAME" && strings.EqualFold(rec.Content, target) {
						app.DNSRecordID = rec.ID
						app.ManageDNS = true
						app.Proxied = rec.Proxied
						break
					}
				}
			}
		}
		if err := s.apps.Create(ctx, app); err != nil {
			res.Skipped = append(res.Skipped, host+": "+err.Error())
			continue
		}
		res.AppsCreated++
	}
	return nil
}
