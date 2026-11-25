package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"cf-multi-tunnel/backend/internal/models"
)

// versionRe extracts the semver from `cloudflared version` output, which looks
// like: "cloudflared version 2025.2.1 (built 2025-02-27-1234 UTC)".
var versionRe = regexp.MustCompile(`version\s+([0-9][0-9A-Za-z.\-]*)`)

// ImageInfo describes the image the panel is configured to run.
type ImageInfo struct {
	Image     string `json:"image"`
	Version   string `json:"version"`
	Digest    string `json:"digest"`
	CheckedAt string `json:"checked_at"`
	Available bool   `json:"available"`
	Error     string `json:"error,omitempty"`
}

type versionCache struct {
	mu      sync.Mutex
	info    *ImageInfo
	fetched time.Time
}

const versionCacheTTL = 2 * time.Minute

// InspectImage reports the version baked into an image. `docker run --version`
// is used instead of image labels because cloudflared does not publish a
// version label; the result is cached because the call costs a container start.
func (s *Supervisor) InspectImage(ctx context.Context, image string) *ImageInfo {
	s.cache.mu.Lock()
	if s.cache.info != nil && s.cache.info.Image == image && time.Since(s.cache.fetched) < versionCacheTTL {
		cached := *s.cache.info
		s.cache.mu.Unlock()
		return &cached
	}
	s.cache.mu.Unlock()

	info := &ImageInfo{Image: image, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	if image == "" {
		info.Error = "no connector image configured"
		return info
	}

	out, err := s.run(ctx, 60*time.Second, "run", "--rm", "--network", "none", "--entrypoint", "cloudflared", image, "version")
	if err != nil {
		info.Error = "image not present locally: " + firstLine(out)
		return info
	}
	if m := versionRe.FindStringSubmatch(out); len(m) == 2 {
		info.Version = m[1]
		info.Available = true
	}

	if digest, err := s.run(ctx, 15*time.Second, "image", "inspect", image, "--format", "{{index .RepoDigests 0}}"); err == nil {
		info.Digest = strings.TrimSpace(digest)
	}

	s.cache.mu.Lock()
	s.cache.info = info
	s.cache.fetched = time.Now()
	s.cache.mu.Unlock()
	return info
}

// InvalidateVersionCache forces the next InspectImage call to re-read Docker,
// which is what the UI needs right after a pull.
func (s *Supervisor) InvalidateVersionCache() {
	s.cache.mu.Lock()
	s.cache.info = nil
	s.cache.fetched = time.Time{}
	s.cache.mu.Unlock()
}

// Pull fetches the image and returns the resulting version.
func (s *Supervisor) Pull(ctx context.Context, image string) (*ImageInfo, error) {
	if image == "" {
		return nil, fmt.Errorf("no connector image configured")
	}
	out, err := s.run(ctx, 10*time.Minute, "pull", image)
	if err != nil {
		return nil, fmt.Errorf("docker pull failed: %s", firstLine(out))
	}
	s.InvalidateVersionCache()
	return s.InspectImage(ctx, image), nil
}

// RunningVersion reads the version from inside a live container, so the UI can
// show what is actually running rather than what the image tag suggests.
func (s *Supervisor) RunningVersion(ctx context.Context, t *models.Tunnel) (string, error) {
	if !t.ManagedLocally() {
		return "", fmt.Errorf("not managed on this host")
	}
	out, err := s.run(ctx, 15*time.Second, "exec", containerName(t), "cloudflared", "version")
	if err != nil {
		return "", err
	}
	if m := versionRe.FindStringSubmatch(out); len(m) == 2 {
		return m[1], nil
	}
	return "", fmt.Errorf("could not parse version from %q", firstLine(out))
}

// ── Remote tag discovery (Docker Hub) ────────────────────────────────

type RemoteTag struct {
	Name      string `json:"name"`
	UpdatedAt string `json:"updated_at"`
	Latest    bool   `json:"latest"`
}

type hubResponse struct {
	Results []struct {
		Name        string `json:"name"`
		LastUpdated string `json:"last_updated"`
	} `json:"results"`
}

// RepoFromImage turns "cloudflare/cloudflared:2025.2.1" into "cloudflare/cloudflared".
func RepoFromImage(image string) string {
	if i := strings.Index(image, "@"); i >= 0 {
		image = image[:i]
	}
	// A colon after the last slash is a tag; a colon before it is a registry port.
	lastSlash := strings.LastIndex(image, "/")
	if i := strings.LastIndex(image, ":"); i > lastSlash {
		image = image[:i]
	}
	parts := strings.Split(image, "/")
	if len(parts) == 3 { // registry/org/repo -> drop the registry
		image = strings.Join(parts[1:], "/")
	}
	if !strings.Contains(image, "/") {
		image = "library/" + image
	}
	return image
}

// ListTags queries Docker Hub for published tags, newest first. Only used to
// populate the "pin a version" dropdown; the panel works offline without it.
func (s *Supervisor) ListTags(ctx context.Context, image string, limit int) ([]RemoteTag, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	repo := RepoFromImage(image)
	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags/?page_size=%d&ordering=last_updated", repo, limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach Docker Hub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Docker Hub returned status %d for %s", resp.StatusCode, repo)
	}

	var payload hubResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	tags := make([]RemoteTag, 0, len(payload.Results))
	for _, item := range payload.Results {
		if strings.HasPrefix(item.Name, "sha256-") {
			continue
		}
		tags = append(tags, RemoteTag{
			Name:      item.Name,
			UpdatedAt: item.LastUpdated,
			Latest:    item.Name == "latest",
		})
	}
	sort.SliceStable(tags, func(i, j int) bool {
		if tags[i].Latest != tags[j].Latest {
			return tags[i].Latest
		}
		return tags[i].UpdatedAt > tags[j].UpdatedAt
	})
	return tags, nil
}

// LatestVersion returns the newest published cloudflared release version, used
// to answer "is an update available?".
func (s *Supervisor) LatestVersion(ctx context.Context, image string) (string, string, error) {
	tags, err := s.ListTags(ctx, image, 50)
	if err != nil {
		return "", "", err
	}
	var newest string
	var updated string
	for _, tag := range tags {
		if !versionRe.MatchString("version " + tag.Name) {
			continue
		}
		// Tags are already ordered newest-first; skip the floating tags.
		if tag.Name == "latest" || tag.Name == "2025.2.0-rc" || strings.Contains(tag.Name, "dev") {
			continue
		}
		newest, updated = tag.Name, tag.UpdatedAt
		break
	}
	return newest, updated, nil
}

func orDefaultLabel(label string) string {
	if strings.TrimSpace(label) == "" {
		return "another device"
	}
	return label
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}
