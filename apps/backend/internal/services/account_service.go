package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cf-multi-tunnel/backend/internal/cloudflare"
	"cf-multi-tunnel/backend/internal/models"
	db "cf-multi-tunnel/backend/internal/repositories/db"
)

type AccountService struct {
	repo       *db.AccountRepository
	zones      *db.ZoneRepository
	tunnels    *db.TunnelRepository
	apps       *db.AppRepository
	cf         *CF
	audit      *AuditService
	conn       ConnectorController
	reconciler TunnelReconciler
}

// TunnelReconciler is implemented by TunnelService. It is injected after
// construction because AccountService is built first; nil is a valid value and
// simply means "no automatic tunnel import".
type TunnelReconciler interface {
	ReconcileAccount(ctx context.Context, caller Caller, accountID string) (*ReconcileResult, error)
}

func (s *AccountService) SetTunnelReconciler(r TunnelReconciler) { s.reconciler = r }

// reconcileAfterToken runs the tunnel + ingress import once an account gains API
// access. A failure is recorded on the account instead of aborting: the token is
// valid and the account is usable, it may just lack one of the four permissions
// the importer needs.
func (s *AccountService) reconcileAfterToken(ctx context.Context, caller Caller, account *models.CFAccount) {
	if s.reconciler == nil {
		return
	}
	res, err := s.reconciler.ReconcileAccount(ctx, caller, account.ID)
	if err != nil {
		account.StatusMessage = "token saved, but importing existing tunnels failed: " + err.Error()
		_ = s.repo.Update(ctx, account)
		return
	}
	if len(res.Skipped) > 0 {
		account.StatusMessage = fmt.Sprintf("imported %d tunnel(s) and %d hostname(s); %d skipped: %s",
			res.Tunnels, res.AppsCreated, len(res.Skipped), res.Skipped[0])
	} else {
		account.StatusMessage = ""
	}
	_ = s.repo.Update(ctx, account)
}

// ConnectorController is the lifecycle hook the account/tunnel services use.
// Keeping it an interface means the docker supervisor is optional: on a host
// without Docker the panel still runs, just with connector_mode=external.
type ConnectorController interface {
	EnsureRunning(ctx context.Context, tunnel *models.Tunnel) error
	Stop(ctx context.Context, tunnel *models.Tunnel) error
	Remove(ctx context.Context, tunnel *models.Tunnel) error
	IsRunning(ctx context.Context, tunnel *models.Tunnel) (bool, error)
	Available() bool
	Logs(ctx context.Context, tunnel *models.Tunnel, lines int) (string, error)
	RecordRunningVersion(ctx context.Context, tunnel *models.Tunnel)
}

func NewAccountService(repo *db.AccountRepository, zones *db.ZoneRepository, tunnels *db.TunnelRepository, apps *db.AppRepository, cf *CF, audit *AuditService, conn ConnectorController) *AccountService {
	return &AccountService{repo: repo, zones: zones, tunnels: tunnels, apps: apps, cf: cf, audit: audit, conn: conn}
}

type CreateAccountInput struct {
	Name      string `json:"name"`
	APIToken  string `json:"api_token"`
	AccountID string `json:"account_id"`
}

// CreateWithToken validates the scoped token, discovers which Cloudflare
// accounts it can reach, and caches the zones so hostname validation is local.
func (s *AccountService) CreateWithToken(ctx context.Context, caller Caller, in CreateAccountInput) (*models.CFAccount, error) {
	token := strings.TrimSpace(in.APIToken)
	if token == "" {
		return nil, Validation(map[string]string{"api_token": "required"})
	}

	client := cloudflare.New(token)
	client.BaseURL = s.cf.cfg.CFAPIBaseURL

	status, err := client.VerifyToken(ctx)
	if err != nil {
		return nil, MapUpstream(err)
	}
	if status.Status != "active" {
		return nil, BadRequest("the API token is " + status.Status + ", not active")
	}

	accounts, err := client.ListAccounts(ctx)
	if err != nil {
		return nil, MapUpstream(err)
	}
	if len(accounts) == 0 {
		return nil, BadRequest("this token cannot read any Cloudflare account; add Account: Account Settings: Read")
	}

	var chosen cloudflare.Account
	switch {
	case in.AccountID != "":
		found := false
		for _, acc := range accounts {
			if acc.ID == in.AccountID {
				chosen, found = acc, true
				break
			}
		}
		if !found {
			return nil, BadRequest("the token does not have access to account " + in.AccountID)
		}
	case len(accounts) == 1:
		chosen = accounts[0]
	default:
		ids := make([]string, 0, len(accounts))
		for _, acc := range accounts {
			ids = append(ids, acc.ID+" ("+acc.Name+")")
		}
		return nil, NewError(409, "account_selection_required",
			"this token can access multiple accounts; choose one in the Cloudflare account list (account_id): "+strings.Join(ids, ", "))
	}

	// Same owner + same Cloudflare account: this is an upsert, not a duplicate.
	// A row created by importing a tunnel token is upgraded in place so its
	// imported tunnels, and everything already configured on them, stay put.
	existing, findErr := s.repo.FindByOwnerAndAccountID(ctx, caller.ID, chosen.ID)
	switch {
	case findErr == nil && existing != nil && existing.Credential == "api":
		return nil, Conflict("you already added account " + chosen.Name)
	case findErr != nil && !errors.Is(findErr, db.ErrNotFound):
		return nil, findErr
	}

	encToken, err := s.cf.Encrypt(token)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = chosen.Name
	}

	if existing != nil {
		existing.Name = name
		existing.AccountName = chosen.Name
		existing.Credential = "api"
		existing.APITokenEnc = encToken
		existing.TokenHint = hint(token)
		existing.Status = "active"
		existing.StatusMessage = ""
		existing.LastCheckedAt = &now
		if err := s.repo.Update(ctx, existing); err != nil {
			return nil, err
		}
		s.syncZones(ctx, existing, client)
		s.reconcileAfterToken(ctx, caller, existing)

		s.audit.Record(ctx, AuditEntry{
			ActorID: caller.ID, ActorEmail: caller.Email, Action: "account.upgrade",
			Entity: "cf_account", EntityID: existing.ID, Detail: existing.AccountID,
		})
		return existing, nil
	}

	account := &models.CFAccount{
		OwnerID:       caller.ID,
		Name:          name,
		AccountID:     chosen.ID,
		AccountName:   chosen.Name,
		Credential:    "api",
		APITokenEnc:   encToken,
		TokenHint:     hint(token),
		Status:        "active",
		LastCheckedAt: &now,
	}
	if err := s.repo.Create(ctx, account); err != nil {
		return nil, err
	}

	s.syncZones(ctx, account, client)
	s.reconcileAfterToken(ctx, caller, account)

	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "account.create",
		Entity: "cf_account", EntityID: account.ID, Detail: account.AccountID,
	})
	return account, nil
}

// AccountChoice is one Cloudflare account a token can reach. The add-account
// form turns this into a combobox, so the user picks a name instead of copying a
// 32-character id out of an error message.
type AccountChoice struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Already bool   `json:"already_added"`
}

// ProbeToken verifies a token and lists the accounts it can reach without
// persisting anything. It exists purely so the form can offer a real choice
// before the user submits.
func (s *AccountService) ProbeToken(ctx context.Context, caller Caller, apiToken string) ([]AccountChoice, error) {
	token := strings.TrimSpace(apiToken)
	if token == "" {
		return nil, Validation(map[string]string{"api_token": "required"})
	}

	client := cloudflare.New(token)
	client.BaseURL = s.cf.cfg.CFAPIBaseURL

	status, err := client.VerifyToken(ctx)
	if err != nil {
		return nil, MapUpstream(err)
	}
	if status.Status != "active" {
		return nil, BadRequest("the API token is " + status.Status + ", not active")
	}

	accounts, err := client.ListAccounts(ctx)
	if err != nil {
		return nil, MapUpstream(err)
	}
	if len(accounts) == 0 {
		return nil, BadRequest("this token cannot read any Cloudflare account; add Account: Account Settings: Read")
	}

	out := make([]AccountChoice, 0, len(accounts))
	for _, acc := range accounts {
		choice := AccountChoice{ID: acc.ID, Name: acc.Name}
		// A token_import row is upgraded in place, so it is not "already added".
		if existing, err := s.repo.FindByOwnerAndAccountID(ctx, caller.ID, acc.ID); err == nil && existing.Credential == "api" {
			choice.Already = true
		}
		out = append(out, choice)
	}
	return out, nil
}

type ImportTunnelInput struct {
	Name          string `json:"name"`
	TunnelToken   string `json:"tunnel_token"`
	Location      string `json:"location"`
	HostLabel     string `json:"host_label"`
	ConnectorMode string `json:"connector_mode"`
}

// ImportTunnelToken is the zero-API-token path: the user pastes the token that
// `cloudflared tunnel token <name>` prints, and the panel derives the account
// and tunnel ids from it. No remote writes are possible until an API token is
// attached, so config_src stays "unknown".
func (s *AccountService) ImportTunnelToken(ctx context.Context, caller Caller, in ImportTunnelInput) (*models.CFAccount, *models.Tunnel, error) {
	payload, err := cloudflare.DecodeTunnelToken(in.TunnelToken)
	if err != nil {
		return nil, nil, BadRequest(err.Error())
	}

	account, err := s.repo.FindByOwnerAndAccountID(ctx, caller.ID, payload.AccountTag)
	if errors.Is(err, db.ErrNotFound) {
		name := strings.TrimSpace(in.Name)
		if name == "" {
			name = "Imported " + payload.AccountTag[:6]
		}
		account = &models.CFAccount{
			OwnerID:       caller.ID,
			Name:          name,
			AccountID:     payload.AccountTag,
			Credential:    "token_import",
			Status:        "active",
			TokenHint:     hint(payload.TunnelSecret),
			StatusMessage: "imported with a tunnel token; attach an API token to manage DNS and ingress from the panel",
		}
		if err := s.repo.Create(ctx, account); err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, err
	}

	encToken, err := s.cf.Encrypt(strings.TrimSpace(in.TunnelToken))
	if err != nil {
		return nil, nil, err
	}
	encSecret, err := s.cf.Encrypt(payload.TunnelSecret)
	if err != nil {
		return nil, nil, err
	}

	mode := in.ConnectorMode
	location := strings.TrimSpace(in.Location)
	if location != "local" && location != "remote" {
		location = "remote"
	}
	if location == "remote" {
		// A tunnel imported by token lives wherever the token holder runs it.
		// Claiming otherwise would make the panel try to supervise a container
		// that does not exist on this host.
		mode = "external"
	} else if mode != "docker" && mode != "external" {
		mode = s.cf.cfg.ConnectorMode
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "imported-" + payload.TunnelID[:8]
	}
	hostLabel := strings.TrimSpace(in.HostLabel)

	tunnel := &models.Tunnel{
		OwnerID:         caller.ID,
		CFAccountID:     account.ID,
		TunnelID:        payload.TunnelID,
		Name:            name,
		Credential:      "token_import",
		TunnelSecretEnc: encSecret,
		TunnelTokenEnc:  encToken,
		ConfigSrc:       "unknown",
		Location:        location,
		HostLabel:       hostLabel,
		ConnectorMode:   mode,
		Status:          "unknown",
	}
	if existing, err := s.tunnels.FindByRemoteID(ctx, account.ID, payload.TunnelID); err == nil && existing != nil {
		return nil, nil, Conflict("this tunnel has already been imported")
	} else if err != nil && !errors.Is(err, db.ErrNotFound) {
		return nil, nil, err
	}
	if err := s.tunnels.Create(ctx, tunnel); err != nil {
		return nil, nil, err
	}

	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "account.import_tunnel",
		Entity: "tunnel", EntityID: tunnel.ID, Detail: payload.TunnelID,
	})
	return account, tunnel, nil
}

func (s *AccountService) List(ctx context.Context, caller Caller) ([]models.CFAccount, error) {
	return s.repo.List(ctx, caller.Actor())
}

func (s *AccountService) Get(ctx context.Context, caller Caller, id string) (*models.CFAccount, error) {
	account, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, NotFound("account not found")
		}
		return nil, err
	}
	if !caller.Admin && account.OwnerID != caller.ID {
		return nil, Forbidden("you do not have access to this account")
	}
	return account, nil
}

func (s *AccountService) Rename(ctx context.Context, caller Caller, id, name string) (*models.CFAccount, error) {
	account, err := s.Get(ctx, caller, id)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, Validation(map[string]string{"name": "required"})
	}
	account.Name = name
	if err := s.repo.Update(ctx, account); err != nil {
		return nil, err
	}
	return account, nil
}

// AttachToken upgrades an imported account to full API management.
func (s *AccountService) AttachToken(ctx context.Context, caller Caller, id, apiToken string) (*models.CFAccount, error) {
	account, err := s.Get(ctx, caller, id)
	if err != nil {
		return nil, err
	}
	apiToken = strings.TrimSpace(apiToken)
	if apiToken == "" {
		return nil, Validation(map[string]string{"api_token": "required"})
	}

	client := cloudflare.New(apiToken)
	client.BaseURL = s.cf.cfg.CFAPIBaseURL
	if _, err := client.VerifyToken(ctx); err != nil {
		return nil, MapUpstream(err)
	}
	accounts, err := client.ListAccounts(ctx)
	if err != nil {
		return nil, MapUpstream(err)
	}
	matched := false
	for _, acc := range accounts {
		if acc.ID == account.AccountID {
			matched = true
			account.AccountName = acc.Name
			break
		}
	}
	if !matched {
		return nil, BadRequest("this token cannot access the account this entry belongs to")
	}

	encToken, err := s.cf.Encrypt(apiToken)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	account.Credential = "api"
	account.APITokenEnc = encToken
	account.TokenHint = hint(apiToken)
	account.Status = "active"
	account.StatusMessage = ""
	account.LastCheckedAt = &now
	if err := s.repo.Update(ctx, account); err != nil {
		return nil, err
	}
	s.syncZones(ctx, account, client)

	// The tunnels imported by token were opaque; now they can be listed,
	// inspected, and their existing ingress imported as published apps.
	s.reconcileAfterToken(ctx, caller, account)

	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "account.attach_token",
		Entity: "cf_account", EntityID: account.ID,
	})
	return account, nil
}

func (s *AccountService) Delete(ctx context.Context, caller Caller, id string) error {
	account, err := s.Get(ctx, caller, id)
	if err != nil {
		return err
	}
	tunnels, err := s.tunnels.ListByAccount(ctx, account.ID)
	if err != nil {
		return err
	}
	// Deleting the account cascades to its tunnels and their published apps. If
	// one of those apps is the panel exposure, the control plane would vanish
	// while the operator still believes it is published.
	for _, tunnel := range tunnels {
		apps, err := s.apps.ListByTunnel(ctx, tunnel.ID)
		if err != nil {
			return err
		}
		for _, app := range apps {
			if app.Kind == "panel" {
				return Conflict("this account publishes the panel itself; move or remove the panel exposure in Settings before deleting it")
			}
		}
	}
	for i := range tunnels {
		_ = s.conn.Stop(ctx, &tunnels[i])
		_ = s.conn.Remove(ctx, &tunnels[i])
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "account.delete",
		Entity: "cf_account", EntityID: id, Detail: account.Name,
	})
	return nil
}

func (s *AccountService) Verify(ctx context.Context, caller Caller, id string) (*models.CFAccount, error) {
	account, err := s.Get(ctx, caller, id)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	account.LastCheckedAt = &now

	if !account.HasAPI() {
		account.Status = "active"
		account.StatusMessage = "tunnel-token account: no API access, add an API token for full management"
		_ = s.repo.Update(ctx, account)
		return account, nil
	}

	client, err := s.cf.ClientFor(account)
	if err != nil {
		return nil, err
	}
	status, err := client.VerifyToken(ctx)
	if err != nil {
		account.Status = "invalid"
		account.StatusMessage = err.Error()
		_ = s.repo.Update(ctx, account)
		return account, MapUpstream(err)
	}
	account.Status = "active"
	account.StatusMessage = ""
	if status.Status != "active" {
		account.Status = "invalid"
		account.StatusMessage = "token status: " + status.Status
	}
	_ = s.repo.Update(ctx, account)
	s.syncZones(ctx, account, client)
	return account, nil
}

func (s *AccountService) SyncZones(ctx context.Context, caller Caller, id string) ([]models.CFZone, error) {
	account, err := s.Get(ctx, caller, id)
	if err != nil {
		return nil, err
	}
	client, err := s.cf.ClientFor(account)
	if err != nil {
		return nil, err
	}
	if err := s.syncZones(ctx, account, client); err != nil {
		return nil, MapUpstream(err)
	}
	return s.zones.ListByAccount(ctx, account.ID)
}

// zoneCacheTTL bounds how long the cached zone directory may go stale. The
// publish-app dropdown reads this list, so a zone created in Cloudflare after
// the last explicit Sync must still show up without the operator thinking to
// press Sync first.
const zoneCacheTTL = 10 * time.Minute

// ListZones returns the cached zone directory for an account, refreshing it
// best-effort when the cache is empty or older than zoneCacheTTL. A failing
// refresh keeps serving the cache: a stale dropdown beats a broken page.
func (s *AccountService) ListZones(ctx context.Context, caller Caller, id string) ([]models.CFZone, error) {
	account, err := s.Get(ctx, caller, id)
	if err != nil {
		return nil, err
	}
	zones, err := s.zones.ListByAccount(ctx, account.ID)
	if err != nil {
		return nil, err
	}
	if len(zones) > 0 && time.Since(zones[0].SyncedAt) < zoneCacheTTL {
		return zones, nil
	}
	client, err := s.cf.ClientFor(account)
	if err != nil {
		return zones, nil
	}
	if err := s.syncZones(ctx, account, client); err != nil {
		return zones, nil
	}
	return s.zones.ListByAccount(ctx, account.ID)
}

// syncZones is best-effort: a token without Zone:Read still leaves a usable
// account, it just cannot auto-create DNS.
func (s *AccountService) syncZones(ctx context.Context, account *models.CFAccount, client *cloudflare.Client) error {
	remote, err := client.ListZones(ctx, account.AccountID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	zones := make([]models.CFZone, 0, len(remote))
	for _, z := range remote {
		zones = append(zones, models.CFZone{
			CFAccountID: account.ID,
			ZoneID:      z.ID,
			Name:        z.Name,
			Status:      z.Status,
			SyncedAt:    now,
		})
	}
	return s.zones.ReplaceForAccount(ctx, account.ID, zones)
}

func hint(secret string) string {
	secret = strings.TrimSpace(secret)
	if len(secret) <= 4 {
		return "****"
	}
	return "****" + secret[len(secret)-4:]
}
