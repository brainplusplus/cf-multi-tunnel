package services

import "testing"

// The publish-app form asks for subdomain + domain separately. An empty
// subdomain is how an operator asks for the apex domain itself, and it used to
// be rejected (the form disabled Publish and the service built nothing).
func TestResolveHostnameAcceptsApexDomain(t *testing.T) {
	cases := []struct {
		name string
		in   CreateAppInput
		want string
	}{
		{"subdomain", CreateAppInput{Subdomain: "app", Domain: "example.com"}, "app.example.com"},
		{"apex", CreateAppInput{Subdomain: "", Domain: "example.com"}, "example.com"},
		{"apex whitespace", CreateAppInput{Subdomain: "   ", Domain: "example.com"}, "example.com"},
		{"explicit hostname wins", CreateAppInput{Hostname: "raw.example.com", Subdomain: "app", Domain: "example.com"}, "raw.example.com"},
		{"uppercase + scheme", CreateAppInput{Subdomain: "", Domain: "HTTPS://Example.COM/"}, "example.com"},
	}
	for _, tc := range cases {
		got, err := resolveHostname(tc.in)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestResolveHostnameRejectsMissingHost(t *testing.T) {
	if _, err := resolveHostname(CreateAppInput{Domain: ""}); err == nil {
		t.Fatal("an empty domain must still be a validation error")
	}
}
