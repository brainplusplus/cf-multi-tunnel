// Package connector owns the cloudflared process/container lifecycle. It is
// deliberately separate from the tunnel service so the panel still runs on a
// host without Docker: in that case every call degrades to a no-op and the
// tunnel is reported as externally managed.
package connector

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"cf-multi-tunnel/backend/internal/config"
	"cf-multi-tunnel/backend/internal/models"
)

// TokenProvider yields the cloudflared token for a tunnel. Implemented by the
// tunnel service; wired after construction to avoid a cycle.
type TokenProvider interface {
	TokenFor(ctx context.Context, t *models.Tunnel) (string, error)
}

type Supervisor struct {
	cfg    *config.Config
	tokens TokenProvider
	images ImageProvider
	mu     sync.Mutex
	cache  versionCache
}

// ImageProvider resolves the active connector image at call time, so an admin
// can pin a new version without restarting the panel.
type ImageProvider func(ctx context.Context) string

func New(cfg *config.Config) *Supervisor {
	return &Supervisor{cfg: cfg}
}

func (s *Supervisor) SetTokenProvider(p TokenProvider) { s.tokens = p }
func (s *Supervisor) SetImageProvider(p ImageProvider) { s.images = p }

func (s *Supervisor) image(ctx context.Context) string {
	if s.images != nil {
		if v := s.images(ctx); v != "" {
			return v
		}
	}
	return s.cfg.CloudflaredImage
}

// DockerVersion reports the daemon version, used as a liveness signal for the
// Connector page.
func (s *Supervisor) DockerVersion() (string, error) {
	out, err := s.run(context.Background(), 15*time.Second, "version", "--format", "{{.Server.Version}}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Available reports whether container management is possible on this host.
func (s *Supervisor) Available() bool {
	out, err := exec.Command(s.cfg.DockerBin, "version", "--format", "{{.Server.Version}}").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

func containerName(t *models.Tunnel) string {
	return "cfmt-" + t.ID
}

func (s *Supervisor) run(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.cfg.DockerBin, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// EnsureRunning starts the tunnel's container if it is not already up. Safe to
// call on every reconcile: it inspects first, so a restart storm cannot happen.
func (s *Supervisor) EnsureRunning(ctx context.Context, t *models.Tunnel) error {
	if !t.ManagedLocally() {
		return nil
	}
	if s.tokens == nil {
		return fmt.Errorf("connector token provider is not configured")
	}
	if running, _ := s.IsRunning(ctx, t); running {
		return nil
	}

	token, err := s.tokens.TokenFor(ctx, t)
	if err != nil {
		return err
	}
	if token == "" {
		return fmt.Errorf("tunnel %s has no runnable token; attach an API token or re-import it", t.Name)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	name := containerName(t)
	// A stopped container with the same name would block `docker run`.
	_, _ = s.run(ctx, 30*time.Second, "rm", "-f", name)

	args := []string{
		"run", "-d",
		"--name", name,
		"--restart", "unless-stopped",
		"--label", "cf-multi-tunnel=1",
		"--label", "cfmt-tunnel-id=" + t.TunnelID,
	}
	if s.cfg.DockerNetwork != "" {
		args = append(args, "--network", s.cfg.DockerNetwork)
	}
	// Let the connector reach services running on this Docker host, so an app
	// published as http://host.docker.internal:8080 works without extra setup.
	// Docker 20.10+ maps the name to the bridge gateway.
	args = append(args, "--add-host", "host.docker.internal:host-gateway")
	args = append(args,
		s.image(ctx),
		"tunnel", "--no-autoupdate", "run", "--token", token,
	)

	out, err := s.run(ctx, 120*time.Second, args...)
	if err != nil {
		return fmt.Errorf("docker run failed: %s: %w", out, err)
	}
	return nil
}

func (s *Supervisor) IsRunning(ctx context.Context, t *models.Tunnel) (bool, error) {
	if !t.ManagedLocally() {
		return false, nil
	}
	out, err := s.run(ctx, 15*time.Second, "inspect", "-f", "{{.State.Running}}", containerName(t))
	if err != nil {
		return false, nil // missing container is not an error, just "not running"
	}
	return strings.TrimSpace(out) == "true", nil
}

func (s *Supervisor) Stop(ctx context.Context, t *models.Tunnel) error {
	if !t.ManagedLocally() {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.run(ctx, 60*time.Second, "stop", "-t", "10", containerName(t)); err != nil {
		// Already stopped or never created: nothing to do.
		return nil
	}
	return nil
}

func (s *Supervisor) Remove(ctx context.Context, t *models.Tunnel) error {
	if !t.ManagedLocally() {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.run(ctx, 60*time.Second, "rm", "-f", containerName(t))
	return nil
}

// Logs returns the tail of the container log, used by the tunnel detail page.
func (s *Supervisor) Logs(ctx context.Context, t *models.Tunnel, lines int) (string, error) {
	if !t.ManagedLocally() {
		if t.Location == "remote" {
			return "", fmt.Errorf("this tunnel runs on %s, not on the panel host; check its logs there", orDefaultLabel(t.HostLabel))
		}
		return "", fmt.Errorf("tunnel is in external mode; run cloudflared yourself")
	}
	if lines <= 0 || lines > 500 {
		lines = 100
	}
	out, err := s.run(ctx, 20*time.Second, "logs", "--tail", fmt.Sprint(lines), containerName(t))
	if err != nil {
		return out, fmt.Errorf("docker logs failed: %w", err)
	}
	return out, nil
}
