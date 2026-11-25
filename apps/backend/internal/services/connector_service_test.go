package services

import (
	"encoding/json"
	"testing"

	"cf-multi-tunnel/backend/internal/connector"
)

// TestConnectorStatusMarshalsEmptyListsAsArrays guards the exact crash the admin
// page showed: "Cannot read properties of null (reading 'length')".
//
// A nil slice marshals to JSON null. The UI reads .running_versions.length, so an
// install with no docker-managed tunnels yet crashed the Connector runtime card.
// The production path goes through newConnectorStatus, which the test uses so a
// regression in that constructor is caught.
func TestConnectorStatusMarshalsEmptyListsAsArrays(t *testing.T) {
	status := newConnectorStatus("docker", true)
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["running_versions"] == nil {
		t.Fatalf("running_versions serialized to null; the UI calls .length on it: %s", raw)
	}
	if _, ok := decoded["running_versions"].([]any); !ok {
		t.Fatalf("running_versions is not a JSON array: %T", decoded["running_versions"])
	}
}

// TestUpdateResultMarshalsEmptyListsAsArrays covers the other half: Pull &
// recreate with zero managed tunnels returned null arrays, which crashed the
// success toast (result.failed.length).
func TestUpdateResultMarshalsEmptyListsAsArrays(t *testing.T) {
	res := newUpdateResult(connector.ImageInfo{Image: "cloudflare/cloudflared:latest"}, "cloudflare/cloudflared:latest")
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"recreated", "failed"} {
		if decoded[key] == nil {
			t.Fatalf("%s serialized to null; the UI calls .length on it: %s", key, raw)
		}
		if _, ok := decoded[key].([]any); !ok {
			t.Fatalf("%s is not a JSON array: %T", key, decoded[key])
		}
	}
}
