package connector

import "testing"

// TestReleaseTagRe pins the tag filter down with the real Docker Hub data that
// broke the "latest version" banner.
//
// cloudflared publishes three kinds of tags per release, ordered by
// last_updated exactly as shown here:
//
//	2016-dc57315806d5        <- git describe, NOT a version
//	2016-dc57315             <- git describe, NOT a version
//	2026.11.2                <- the actual release
//	latest                   <- floating
//	2026.11.2-amd64          <- arch variant of the release
//
// The previous check ("starts with a digit") matched the git-describe tags and
// showed LATEST = 2016-dc57315806d5 in the UI. Only the CalVer shape is right.
func TestReleaseTagRe(t *testing.T) {
	cases := []struct {
		tag  string
		want bool
	}{
		{"2026.11.2", true},
		{"2026.11.1", true},
		{"2025.2.1", true},
		{"2026.1.0", true},
		{"2016-dc57315806d5", false}, // git describe
		{"2016-dc57315", false},      // git describe, short form
		{"2012-518d7f7969b7", false},
		{"latest", false},
		{"2026.11.2-amd64", false}, // arch variants are not the release tag
		{"2026.11.2-arm64", false},
		{"dev", false},
		{"2026.11", false},
	}
	for _, c := range cases {
		if got := releaseTagRe.MatchString(c.tag); got != c.want {
			t.Errorf("releaseTagRe.MatchString(%q) = %v, want %v", c.tag, got, c.want)
		}
	}
}

// TestLatestVersionPicksRealRelease replays the first page of Docker Hub
// results through the production selector and asserts the panel reports
// 2026.11.2, not the git-describe tag. It calls pickLatestRelease — the exact
// function LatestVersion uses — so a regression in the filter fails here.
func TestLatestVersionPicksRealRelease(t *testing.T) {
	tags := []RemoteTag{
		{Name: "2016-dc57315806d5", UpdatedAt: "2026-11-23T13:18:42Z"},
		{Name: "2016-dc57315", UpdatedAt: "2026-11-23T13:18:38Z"},
		{Name: "2026.11.2", UpdatedAt: "2026-11-23T13:18:34Z"},
		{Name: "latest", UpdatedAt: "2026-11-23T13:18:30Z", Latest: true},
		{Name: "2016-dc57315806d5-arm64", UpdatedAt: "2026-11-23T13:18:25Z"},
		{Name: "2016-dc57315-arm64", UpdatedAt: "2026-11-23T13:18:23Z"},
		{Name: "2026.11.2-arm64", UpdatedAt: "2026-11-23T13:18:20Z"},
		{Name: "latest-arm64", UpdatedAt: "2026-11-23T13:18:18Z"},
	}

	name, updated := pickLatestRelease(tags)
	if name != "2026.11.2" {
		t.Fatalf("newest release = %q, want 2026.11.2", name)
	}
	if updated != "2026-11-23T13:18:34Z" {
		t.Fatalf("updated = %q, want the release's own timestamp", updated)
	}
}

// TestPickLatestReleaseIgnoresNonReleases makes sure a page containing only
// git-describe/arch tags reports nothing instead of a bogus version.
func TestPickLatestReleaseIgnoresNonReleases(t *testing.T) {
	name, _ := pickLatestRelease([]RemoteTag{
		{Name: "latest"},
		{Name: "2016-dc57315806d5"},
		{Name: "2026.11.2-amd64"},
	})
	if name != "" {
		t.Fatalf("expected no release, got %q", name)
	}
}
