package services

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"cf-multi-tunnel/backend/internal/config"
	"cf-multi-tunnel/backend/internal/connector"
	"cf-multi-tunnel/backend/internal/models"
	db "cf-multi-tunnel/backend/internal/repositories/db"
)

// connectorImageKey is the settings row that overrides CLOUDFLARED_IMAGE.
const connectorImageKey = "connector.image"

type ConnectorService struct {
	sup      *connector.Supervisor
	settings *db.SettingRepository
	tunnels  *db.TunnelRepository
	cfg      *config.Config
	audit    *AuditService
	recreate sync.Mutex
}

func NewConnectorService(sup *connector.Supervisor, settings *db.SettingRepository, tunnels *db.TunnelRepository, cfg *config.Config, audit *AuditService) *ConnectorService {
	return &ConnectorService{sup: sup, settings: settings, tunnels: tunnels, cfg: cfg, audit: audit}
}

// Image resolves the active image: DB override first, then env default.
func (s *ConnectorService) Image(ctx context.Context) string {
	if v, err := s.settings.Get(ctx, connectorImageKey); err == nil && strings.TrimSpace(v) != "" {
		return v
	}
	return s.cfg.CloudflaredImage
}

type ConnectorStatus struct {
	Mode            string               `json:"mode"`
	DockerAvailable bool                 `json:"docker_available"`
	DockerVersion   string               `json:"docker_version"`
	Image           connector.ImageInfo  `json:"image"`
	LatestVersion   string               `json:"latest_version"`
	UpdateAvailable bool                 `json:"update_available"`
	ManagedTunnels  int                  `json:"managed_tunnels"`
	RunningTunnels  int                  `json:"running_tunnels"`
	RunningVersions []TunnelImageVersion `json:"running_versions"`
	LastCheckedAt   string               `json:"last_checked_at"`
}

type TunnelImageVersion struct {
	TunnelID string `json:"tunnel_id"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	ImageRef string `json:"image_ref"`
}

// newConnectorStatus builds a status with non-nil slices. A nil slice marshals
// to JSON null and the UI calls .running_versions.length, so every construction
// must go through here.
func newConnectorStatus(mode string, dockerAvailable bool) *ConnectorStatus {
	return &ConnectorStatus{
		Mode:            mode,
		DockerAvailable: dockerAvailable,
		LastCheckedAt:   time.Now().UTC().Format(time.RFC3339),
		RunningVersions: []TunnelImageVersion{},
	}
}

// Status is what the admin "Connector" page renders: what we run, what is
// newest, and whether an update is available.
func (s *ConnectorService) Status(ctx context.Context, checkRemote bool) (*ConnectorStatus, error) {
	image := s.Image(ctx)
	status := newConnectorStatus(s.cfg.ConnectorMode, s.sup.Available())
	if !status.DockerAvailable {
		status.Image = connector.ImageInfo{Image: image, Error: "docker not reachable from the panel"}
		return status, nil
	}

	status.Image = *s.sup.InspectImage(ctx, image)
	if out, err := s.sup.DockerVersion(); err == nil {
		status.DockerVersion = out
	}

	tunnels, err := s.tunnels.List(ctx, db.Actor{Admin: true}, "", "")
	if err != nil {
		return nil, err
	}
	for i := range tunnels {
		if !tunnels[i].ManagedLocally() {
			continue
		}
		status.ManagedTunnels++
		if tunnels[i].Status == "running" {
			status.RunningTunnels++
		}
		status.RunningVersions = append(status.RunningVersions, TunnelImageVersion{
			TunnelID: tunnels[i].ID,
			Name:     tunnels[i].Name,
			Version:  tunnels[i].ImageVersion,
			ImageRef: tunnels[i].ImageRef,
		})
	}

	if checkRemote {
		if latest, _, err := s.sup.LatestVersion(ctx, image); err == nil && latest != "" {
			status.LatestVersion = latest
			status.UpdateAvailable = status.Image.Version != "" && latest != status.Image.Version
		}
	}
	return status, nil
}

func (s *ConnectorService) ListTags(ctx context.Context, limit int) ([]connector.RemoteTag, error) {
	return s.sup.ListTags(ctx, s.Image(ctx), limit)
}

type UpdateResult struct {
	Image         connector.ImageInfo `json:"image"`
	Recreated     []string            `json:"recreated"`
	Failed        []string            `json:"failed"`
	PreviousImage string              `json:"previous_image"`
}

// newUpdateResult builds a result with non-nil slices: nil marshals to JSON null
// and the UI reads .recreated.length / .failed.length.
func newUpdateResult(image connector.ImageInfo, previous string) *UpdateResult {
	return &UpdateResult{
		Image:         image,
		PreviousImage: previous,
		Recreated:     []string{},
		Failed:        []string{},
	}
}

// Update pulls the target image, optionally persists it as the new default, and
// recreates every docker-managed tunnel. Recreate is serialised: two concurrent
// updates would fight over the same container names.
func (s *ConnectorService) Update(ctx context.Context, caller Caller, imageRef string, pin bool) (*UpdateResult, error) {
	if !caller.Admin {
		return nil, Forbidden("only an admin can update the connector")
	}
	if !s.sup.Available() {
		return nil, NewError(503, "docker_unavailable", "Docker is not reachable from the panel")
	}

	target := strings.TrimSpace(imageRef)
	if target == "" {
		target = s.Image(ctx)
	}
	previous := s.Image(ctx)

	pulled, err := s.sup.Pull(ctx, target)
	if err != nil {
		return nil, Upstream(err)
	}

	if pin {
		if err := s.settings.Set(ctx, connectorImageKey, target); err != nil {
			return nil, err
		}
	}

	s.recreate.Lock()
	defer s.recreate.Unlock()

	// Empty slices, not nil: a nil slice marshals to JSON null and the UI reads
	// .length on both fields, which threw "Cannot read properties of null" on
	// an install with no docker-managed tunnels yet.
	result := newUpdateResult(*pulled, previous)
	tunnels, err := s.tunnels.List(ctx, db.Actor{Admin: true}, "", "")
	if err != nil {
		return nil, err
	}
	for i := range tunnels {
		t := &tunnels[i]
		if !t.ManagedLocally() || !t.DesiredRunning {
			continue
		}
		t.ImageRef = target
		if err := s.sup.EnsureRunning(ctx, t); err != nil {
			result.Failed = append(result.Failed, t.Name+": "+err.Error())
			t.Status = "error"
			t.StatusMessage = err.Error()
			_ = s.tunnels.Update(ctx, t)
			continue
		}
		if version, err := s.sup.RunningVersion(ctx, t); err == nil {
			t.ImageVersion = version
		} else {
			t.ImageVersion = pulled.Version
		}
		t.Status = "running"
		t.StatusMessage = ""
		_ = s.tunnels.Update(ctx, t)
		result.Recreated = append(result.Recreated, t.Name)
	}

	s.audit.Record(ctx, AuditEntry{
		ActorID: caller.ID, ActorEmail: caller.Email, Action: "connector.update",
		Entity: "connector", EntityID: target,
		Detail: previous + " -> " + target + " (" + pulled.Version + ")",
	})
	slog.Info("connector updated", "image", target, "version", pulled.Version,
		"recreated", len(result.Recreated), "failed", len(result.Failed))
	return result, nil
}

// EnsureRunningFor is called after a tunnel starts so the stored image version
// matches reality without an extra admin click.
func (s *ConnectorService) RecordRunningVersion(ctx context.Context, tunnel *models.Tunnel) {
	if !tunnel.ManagedLocally() {
		return
	}
	tunnel.ImageRef = s.Image(ctx)
	if version, err := s.sup.RunningVersion(ctx, tunnel); err == nil {
		tunnel.ImageVersion = version
	} else {
		tunnel.ImageVersion = s.sup.InspectImage(ctx, tunnel.ImageRef).Version
	}
	_ = s.tunnels.Update(ctx, tunnel)
}

// ── Supervisor facade ────────────────────────────────────────────────
// ConnectorService is the single object the other services talk to, so the
// image-version logic and the lifecycle logic cannot drift apart.

func (s *ConnectorService) EnsureRunning(ctx context.Context, t *models.Tunnel) error {
	return s.sup.EnsureRunning(ctx, t)
}

func (s *ConnectorService) Stop(ctx context.Context, t *models.Tunnel) error {
	return s.sup.Stop(ctx, t)
}

func (s *ConnectorService) Remove(ctx context.Context, t *models.Tunnel) error {
	return s.sup.Remove(ctx, t)
}

func (s *ConnectorService) IsRunning(ctx context.Context, t *models.Tunnel) (bool, error) {
	return s.sup.IsRunning(ctx, t)
}

func (s *ConnectorService) Available() bool { return s.sup.Available() }

func (s *ConnectorService) Logs(ctx context.Context, t *models.Tunnel, lines int) (string, error) {
	return s.sup.Logs(ctx, t, lines)
}
