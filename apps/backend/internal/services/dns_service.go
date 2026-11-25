package services

import (
	"context"
	"errors"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"cf-multi-tunnel/backend/internal/cloudflare"
	"cf-multi-tunnel/backend/internal/models"
	db "cf-multi-tunnel/backend/internal/repositories/db"
)

// DNSService backs the DNS page. Records are read live from Cloudflare instead
// of cached: a stale copy of a zone is worse than a slow list, because deleting
// a record the panel no longer shows is a silent outage. Only the zone list is
// cached, as a directory of what can be opened.
type DNSService struct {
	accounts   *db.AccountRepository
	zones      *db.ZoneRepository
	tunnels    *db.TunnelRepository
	apps       *db.AppRepository
	accountSvc *AccountService
	cf         *CF
	audit      *AuditService
}

func NewDNSService(
	accounts *db.AccountRepository,
	zones *db.ZoneRepository,
	tunnels *db.TunnelRepository,
	apps *db.AppRepository,
	accountSvc *AccountService,
	cf *CF,
	audit *AuditService,
) *DNSService {
	return &DNSService{
		accounts: accounts, zones: zones, tunnels: tunnels, apps: apps,
		accountSvc: accountSvc, cf: cf, audit: audit,
	}
}

// DNSZone is a cached zone plus the account it belongs to, so the page can offer
// one dropdown instead of a request per account.
type DNSZone struct {
	ID          string    `json:"id"`
	ZoneID      string    `json:"zone_id"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	CFAccountID string    `json:"cf_account_id"`
	AccountName string    `json:"account_name"`
	HasAPI      bool      `json:"has_api"`
	SyncedAt    time.Time `json:"synced_at"`
}

// DNSRecordView is a Cloudflare record plus the facts the panel adds: who owns
// it, and how its type should read ("Tunnel" for a CNAME that points at
// cfargotunnel.com, which is how the Cloudflare dashboard labels it).
type DNSRecordView struct {
	cloudflare.DNSRecord
	ZoneID      string `json:"zone_id"`
	ZoneName    string `json:"zone_name"`
	DisplayType string `json:"display_type"`
	Managed     bool   `json:"managed"`
	TunnelID    string `json:"tunnel_id,omitempty"`
	TunnelName  string `json:"tunnel_name,omitempty"`
	AppID       string `json:"app_id,omitempty"`
	Hostname    string `json:"hostname,omitempty"`
}

type DNSSyncResult struct {
	Accounts int      `json:"accounts"`
	Zones    int      `json:"zones"`
	Failed   []string `json:"failed"`
}

// ListZones returns the cached zones of every account the caller can reach.
func (s *DNSService) ListZones(ctx context.Context, caller Caller) ([]DNSZone, error) {
	accounts, err := s.accounts.List(ctx, caller.Actor())
	if err != nil {
		return nil, err
	}
	zones, err := s.zones.List(ctx)
	if err != nil {
		return nil, err
	}

	visible := make(map[string]models.CFAccount, len(accounts))
	for _, account := range accounts {
		visible[account.ID] = account
	}

	out := make([]DNSZone, 0, len(zones))
	for _, zone := range zones {
		account, ok := visible[zone.CFAccountID]
		if !ok {
			continue
		}
		out = append(out, DNSZone{
			ID: zone.ID, ZoneID: zone.ZoneID, Name: zone.Name, Status: zone.Status,
			CFAccountID: account.ID, AccountName: account.Name,
			HasAPI: account.HasAPI(), SyncedAt: zone.SyncedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// SyncZones refreshes the cached zone list for every account with an API token.
// One failing account must not stop the rest, so failures are collected and
// reported instead of returned.
func (s *DNSService) SyncZones(ctx context.Context, caller Caller) (*DNSSyncResult, error) {
	accounts, err := s.accounts.List(ctx, caller.Actor())
	if err != nil {
		return nil, err
	}

	result := &DNSSyncResult{Failed: []string{}}
	for _, account := range accounts {
		if !account.HasAPI() {
			continue
		}
		result.Accounts++
		zones, err := s.accountSvc.SyncZones(ctx, caller, account.ID)
		if err != nil {
			result.Failed = append(result.Failed, account.Name+": "+err.Error())
			continue
		}
		result.Zones += len(zones)
	}

	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "dns.zones_sync",
		Entity: "zone", EntityID: "",
		Detail: "refreshed " + strconv.Itoa(result.Zones) + " zone(s) from " + strconv.Itoa(result.Accounts) + " account(s)",
	})
	return result, nil
}

// ListRecords reads a zone live from Cloudflare and annotates every record with
// the published app that owns it, so the UI can lock those rows instead of
// letting a delete break a tunnel.
func (s *DNSService) ListRecords(ctx context.Context, caller Caller, zoneRowID string) ([]DNSRecordView, bool, error) {
	zone, _, client, err := s.zoneAccess(ctx, caller, zoneRowID)
	if err != nil {
		return nil, false, err
	}

	records, truncated, err := client.ListAllDNSRecords(ctx, zone.ZoneID)
	if err != nil {
		return nil, false, MapUpstream(err)
	}
	apps, err := s.apps.List(ctx, caller.Actor())
	if err != nil {
		return nil, false, err
	}

	views := make([]DNSRecordView, 0, len(records))
	for i := range records {
		views = append(views, s.viewForRecord(ctx, zone, &records[i], apps))
	}
	return views, truncated, nil
}

// CreateRecord writes a record straight to Cloudflare. A hostname already
// published as a CNAME by an app is refused, because two owners for one name
// would break the tunnel without the app ever knowing.
func (s *DNSService) CreateRecord(ctx context.Context, caller Caller, zoneRowID string, in DNSRecordInput) (*DNSRecordView, error) {
	zone, _, client, err := s.zoneAccess(ctx, caller, zoneRowID)
	if err != nil {
		return nil, err
	}
	payload, err := buildDNSRecordInput(zone.Name, in)
	if err != nil {
		return nil, err
	}
	apps, err := s.apps.List(ctx, caller.Actor())
	if err != nil {
		return nil, err
	}
	if payload.Type == "CNAME" && appForHostname(apps, payload.Name) != nil {
		return nil, Conflict(payload.Name + " is published by a tunnel app; change it from that tunnel instead of adding a second record")
	}

	created, err := client.CreateDNSRecord(ctx, zone.ZoneID, payload)
	if err != nil {
		return nil, MapUpstream(err)
	}
	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "dns.create",
		Entity: "dns_record", EntityID: created.ID, Detail: describeRecord(zone.Name, created),
	})

	view := s.viewForRecord(ctx, zone, created, apps)
	return &view, nil
}

// UpdateRecord refuses to touch a record a published app owns. Editing it here
// would desync the app row, and the next publish would silently overwrite it.
func (s *DNSService) UpdateRecord(ctx context.Context, caller Caller, zoneRowID, recordID string, in DNSRecordInput) (*DNSRecordView, error) {
	zone, _, client, err := s.zoneAccess(ctx, caller, zoneRowID)
	if err != nil {
		return nil, err
	}
	existing, err := s.findRecord(ctx, client, zone.ZoneID, recordID)
	if err != nil {
		return nil, err
	}
	apps, err := s.apps.List(ctx, caller.Actor())
	if err != nil {
		return nil, err
	}
	if app := appForRecord(apps, zone.ZoneID, existing); app != nil {
		return nil, Conflict(managedMessage(app))
	}
	payload, err := buildDNSRecordInput(zone.Name, in)
	if err != nil {
		return nil, err
	}
	if payload.Type == "CNAME" && payload.Name != existing.Name {
		if app := appForHostname(apps, payload.Name); app != nil {
			return nil, Conflict(payload.Name + " is published by a tunnel app; change it from that tunnel instead")
		}
	}

	updated, err := client.UpdateDNSRecord(ctx, zone.ZoneID, recordID, payload)
	if err != nil {
		return nil, MapUpstream(err)
	}
	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "dns.update",
		Entity: "dns_record", EntityID: updated.ID, Detail: describeRecord(zone.Name, updated),
	})

	view := s.viewForRecord(ctx, zone, updated, apps)
	return &view, nil
}

func (s *DNSService) DeleteRecord(ctx context.Context, caller Caller, zoneRowID, recordID string) error {
	zone, _, client, err := s.zoneAccess(ctx, caller, zoneRowID)
	if err != nil {
		return err
	}
	existing, err := s.findRecord(ctx, client, zone.ZoneID, recordID)
	if err != nil {
		return err
	}
	apps, err := s.apps.List(ctx, caller.Actor())
	if err != nil {
		return err
	}
	if app := appForRecord(apps, zone.ZoneID, existing); app != nil {
		return Conflict(managedMessage(app))
	}

	if err := client.DeleteDNSRecord(ctx, zone.ZoneID, recordID); err != nil {
		return MapUpstream(err)
	}
	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "dns.delete",
		Entity: "dns_record", EntityID: recordID, Detail: describeRecord(zone.Name, existing),
	})
	return nil
}

// ── Record input ─────────────────────────────────────────────────────

// DNSRecordInput is the wire format for the DNS page. Type-specific fields are
// spelled out instead of a free-form data object so the form can render the
// right inputs and the backend can validate them.
type DNSRecordInput struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Proxied  *bool  `json:"proxied"`
	Priority *int   `json:"priority"`
	Comment  string `json:"comment"`

	// SRV: the owner is built from service + protocol + zone.
	Service  string `json:"service"`
	Protocol string `json:"protocol"`
	Weight   *int   `json:"weight"`
	Port     *int   `json:"port"`
	Target   string `json:"target"`

	// CAA
	Flags *int   `json:"flags"`
	Tag   string `json:"tag"`
	Value string `json:"value"`
}

var dnsRecordTypes = map[string]bool{
	"A": true, "AAAA": true, "CNAME": true, "TXT": true,
	"MX": true, "NS": true, "SRV": true, "CAA": true,
}

// proxiableTypes are the records Cloudflare can put behind the proxy. Sending
// Proxied on a TXT record is an API error, so it is only ever set for these.
var proxiableTypes = map[string]bool{"A": true, "AAAA": true, "CNAME": true}

func (s *DNSService) zoneAccess(ctx context.Context, caller Caller, zoneRowID string) (*models.CFZone, *models.CFAccount, *cloudflare.Client, error) {
	zone, err := s.zones.FindByID(ctx, zoneRowID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, nil, nil, NotFound("zone not found; sync the account zones first")
		}
		return nil, nil, nil, err
	}
	account, err := s.accounts.FindByID(ctx, zone.CFAccountID)
	if err != nil {
		return nil, nil, nil, err
	}
	if !caller.Admin && account.OwnerID != caller.ID {
		return nil, nil, nil, Forbidden("you do not have access to this zone")
	}
	client, err := s.cf.ClientFor(account)
	if err != nil {
		return nil, nil, nil, err
	}
	return zone, account, client, nil
}

func (s *DNSService) findRecord(ctx context.Context, client *cloudflare.Client, zoneID, recordID string) (*cloudflare.DNSRecord, error) {
	records, _, err := client.ListAllDNSRecords(ctx, zoneID)
	if err != nil {
		return nil, MapUpstream(err)
	}
	for i := range records {
		if records[i].ID == recordID {
			return &records[i], nil
		}
	}
	return nil, NotFound("dns record not found; it may have been deleted in Cloudflare")
}

// viewForRecord marks a row as tunnel-owned when a published app owns that exact
// record id, or when the name is published and its CNAME targets a tunnel. The
// second check covers DNS recreated outside the panel: the name is still owned
// even when Cloudflare issued a new record id.
func (s *DNSService) viewForRecord(ctx context.Context, zone *models.CFZone, rec *cloudflare.DNSRecord, apps []models.PublishedApp) DNSRecordView {
	view := DNSRecordView{
		DNSRecord: *rec, ZoneID: zone.ZoneID, ZoneName: zone.Name, DisplayType: rec.Type,
	}
	if rec.Type == "CNAME" && strings.HasSuffix(strings.ToLower(rec.Content), ".cfargotunnel.com") {
		view.DisplayType = "Tunnel"
		view.TunnelID = strings.TrimSuffix(strings.ToLower(rec.Content), ".cfargotunnel.com")
		if tunnel, err := s.tunnels.FindByRemoteID(ctx, zone.CFAccountID, view.TunnelID); err == nil && tunnel != nil {
			view.TunnelName = tunnel.Name
		}
	}
	if app := appForRecord(apps, zone.ZoneID, rec); app != nil {
		view.Managed = true
		view.AppID = app.ID
		view.Hostname = app.Hostname
	}
	return view
}

func appForRecord(apps []models.PublishedApp, zoneID string, rec *cloudflare.DNSRecord) *models.PublishedApp {
	for i := range apps {
		app := &apps[i]
		if app.DNSRecordID != "" && app.DNSRecordID == rec.ID {
			return app
		}
	}
	if rec.Type != "CNAME" || !strings.HasSuffix(strings.ToLower(rec.Content), ".cfargotunnel.com") {
		return nil
	}
	for i := range apps {
		app := &apps[i]
		if app.ZoneID == zoneID && strings.EqualFold(app.Hostname, rec.Name) {
			return app
		}
	}
	return nil
}

func appForHostname(apps []models.PublishedApp, hostname string) *models.PublishedApp {
	for i := range apps {
		if strings.EqualFold(apps[i].Hostname, hostname) {
			return &apps[i]
		}
	}
	return nil
}

func managedMessage(app *models.PublishedApp) string {
	return app.Hostname + " is managed by a tunnel app; change it from the tunnel page so ingress and DNS stay in sync"
}

func describeRecord(zoneName string, rec *cloudflare.DNSRecord) string {
	detail := rec.Type + " " + rec.Name
	if rec.Content != "" {
		detail += " → " + rec.Content
	}
	return detail + " (" + zoneName + ")"
}

// ── Validation ───────────────────────────────────────────────────────

var (
	srvOwnerRe = regexp.MustCompile(`^_[a-z0-9-]+\._(tcp|udp|tls)$`)
	maxTXT     = 2048
)

// buildDNSRecordInput normalizes and validates one submitted record. Anything
// Cloudflare would reject with a raw API error is caught here with a field error
// the form can highlight.
func buildDNSRecordInput(zoneName string, in DNSRecordInput) (cloudflare.DNSRecordInput, error) {
	fields := map[string]string{}
	recType := strings.ToUpper(strings.TrimSpace(in.Type))
	if !dnsRecordTypes[recType] {
		return cloudflare.DNSRecordInput{}, Validation(map[string]string{
			"type": "pick a record type (A, AAAA, CNAME, TXT, MX, NS, SRV, CAA)",
		})
	}

	out := cloudflare.DNSRecordInput{
		Type: recType, TTL: in.TTL, Comment: strings.TrimSpace(in.Comment),
	}
	if in.TTL < 0 {
		fields["ttl"] = "TTL cannot be negative"
	}

	if recType == "SRV" {
		service := strings.ToLower(strings.TrimSpace(in.Service))
		proto := strings.ToLower(strings.TrimSpace(in.Protocol))
		owner := service + "." + proto
		if service == "" || proto == "" {
			fields["service"] = "fill service and protocol, e.g. _sip._tcp"
		} else if !srvOwnerRe.MatchString(owner) {
			fields["service"] = "use the form _service._tcp (or _udp/_tls)"
		}
		target := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(in.Target)), ".")
		if target == "" {
			fields["target"] = "enter the target hostname"
		} else if _, err := NormalizeHostname(target); err != nil {
			fields["target"] = err.Error()
		}
		port, weight, priority := valueOr(in.Port, 0), valueOr(in.Weight, 0), valueOr(in.Priority, 0)
		if port < 1 || port > 65535 {
			fields["port"] = "port must be between 1 and 65535"
		}
		if weight < 0 || weight > 65535 {
			fields["weight"] = "weight must be between 0 and 65535"
		}
		if priority < 0 || priority > 65535 {
			fields["priority"] = "priority must be between 0 and 65535"
		}
		out.Name = owner + "." + strings.TrimSuffix(strings.ToLower(zoneName), ".")
		out.Data = &cloudflare.DNSRecordData{Priority: &priority, Weight: &weight, Port: &port, Target: target}
	} else {
		name, err := dnsRecordName(zoneName, in.Name)
		if err != nil {
			fields["name"] = err.Error()
		}
		out.Name = name

		switch recType {
		case "A", "AAAA":
			content := strings.TrimSpace(in.Content)
			ip := net.ParseIP(content)
			switch {
			case ip == nil:
				fields["content"] = "enter the IP address this name should resolve to"
			case recType == "A" && ip.To4() == nil:
				fields["content"] = "an A record needs an IPv4 address"
			case recType == "AAAA" && ip.To4() != nil:
				fields["content"] = "an AAAA record needs an IPv6 address"
			default:
				out.Content = content
			}
		case "CNAME", "NS":
			target := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(in.Content)), ".")
			if target == "" {
				fields["content"] = "enter a target hostname"
			} else if _, err := NormalizeHostname(target); err != nil {
				fields["content"] = err.Error()
			} else {
				out.Content = target
			}
		case "TXT":
			content := strings.TrimSpace(in.Content)
			if content == "" {
				fields["content"] = "enter the record text"
			} else if len(content) > maxTXT {
				fields["content"] = "TXT record is too long (max 2048 characters)"
			} else {
				out.Content = content
			}
		case "MX":
			target := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(in.Content)), ".")
			if target == "" {
				fields["content"] = "enter the mail server hostname"
			} else if _, err := NormalizeHostname(target); err != nil {
				fields["content"] = err.Error()
			} else {
				out.Content = target
			}
			priority := valueOr(in.Priority, 10)
			if priority < 0 || priority > 65535 {
				fields["priority"] = "priority must be between 0 and 65535"
			} else {
				out.Priority = &priority
			}
		case "CAA":
			flags := valueOr(in.Flags, 0)
			if flags < 0 || flags > 255 {
				fields["flags"] = "flags must be between 0 and 255"
			}
			tag := strings.ToLower(strings.TrimSpace(in.Tag))
			if tag == "" {
				fields["tag"] = "enter a tag (issue, issuewild, iodef)"
			}
			value := strings.TrimSpace(in.Value)
			if value == "" {
				fields["value"] = "enter the tag value"
			}
			out.Data = &cloudflare.DNSRecordData{Flags: &flags, Tag: tag, Value: value}
		}
	}

	if proxiableTypes[recType] {
		proxied := true
		if in.Proxied != nil {
			proxied = *in.Proxied
		}
		out.Proxied = &proxied
	} else if in.Proxied != nil && *in.Proxied {
		fields["proxied"] = "only A, AAAA and CNAME records can be proxied"
	}

	if len(fields) > 0 {
		return cloudflare.DNSRecordInput{}, Validation(fields)
	}
	return out, nil
}

// dnsRecordName accepts "@", a bare label, a multi-label relative name, or a
// full name already inside the zone. The UI shows the zone suffix next to the
// input, so a value without the suffix is always read as relative.
func dnsRecordName(zoneName, raw string) (string, error) {
	zone := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(zoneName)), ".")
	value := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if zone == "" {
		return "", errors.New("pick a zone first")
	}
	if value == "" || value == "@" || value == zone {
		return zone, nil
	}
	if strings.HasSuffix(value, "."+zone) {
		if !validRelativeName(strings.TrimSuffix(value, "."+zone)) {
			return "", errors.New("use letters, digits, hyphens and underscores only")
		}
		return value, nil
	}
	if !validRelativeName(value) {
		return "", errors.New("use letters, digits, hyphens and underscores only")
	}
	return value + "." + zone, nil
}

func validRelativeName(name string) bool {
	if name == "" || len(name) > 200 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if label == "*" {
			continue
		}
		if label == "" || len(label) > 63 {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			ok := c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_'
			if !ok {
				return false
			}
		}
	}
	return true
}

func valueOr(v *int, fallback int) int {
	if v == nil {
		return fallback
	}
	return *v
}
