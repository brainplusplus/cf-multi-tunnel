package services

import "testing"

// A LAN origin is normally typed as "192.168.0.103:80" or "192.168.0.103".
// Cloudflare requires a scheme, so the panel adds http:// instead of failing
// with a URL parse error the operator cannot act on.
func TestValidateServiceURLAddsMissingScheme(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"192.168.0.103:80", "http://192.168.0.103:80"},
		{"192.168.0.103", "http://192.168.0.103"},
		{"host.docker.internal:8080", "http://host.docker.internal:8080"},
		{"localhost:3000", "http://localhost:3000"},
	}
	for _, c := range cases {
		got, err := ValidateServiceURL(c.in)
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("%q: got %q, want %q", c.in, got, c.want)
		}
	}
}

// An explicit scheme must be preserved, including the ones that are not http.
func TestValidateServiceURLKeepsExplicitScheme(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://192.168.0.103", "https://192.168.0.103"},
		{"http://192.168.0.103:80", "http://192.168.0.103:80"},
		{"tcp://192.168.0.103:5432", "tcp://192.168.0.103:5432"},
		{"ssh://192.168.0.103:22", "ssh://192.168.0.103:22"},
		{"unix:///var/run/app.sock", "unix:///var/run/app.sock"},
	}
	for _, c := range cases {
		got, err := ValidateServiceURL(c.in)
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("%q: got %q, want %q", c.in, got, c.want)
		}
	}
}

// The normalisation must not turn garbage into something publishable.
func TestValidateServiceURLRejectsGarbage(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"gopher://example.com",
		"ftp://example.com",
	}
	for _, in := range cases {
		if got, err := ValidateServiceURL(in); err == nil {
			t.Fatalf("%q was accepted as %q, want an error", in, got)
		}
	}
}
