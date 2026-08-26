package instance

import (
	"bytes"
	"testing"
)

func TestWriteReadJSON_RoundTrip(t *testing.T) {
	s := &Snapshot{
		GeneratedAt: "2026-01-01T00:00:00Z",
		System:      SystemInfo{ArtifactoryVersion: "7.165.0", Addons: []string{"docker", "helmoci"}},
		Security:    SecurityInfo{OIDCProviders: []OIDCProvider{{Name: "github", ProviderType: "GitHub"}}},
		Users:       []User{{Username: "alice", Realm: "internal"}},
		Warnings:    []string{"something soft-failed"},
	}

	var buf bytes.Buffer
	if err := WriteJSON(&buf, s); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}

	got, err := ReadJSON(&buf)
	if err != nil {
		t.Fatalf("ReadJSON returned error: %v", err)
	}
	if got.System.ArtifactoryVersion != "7.165.0" {
		t.Errorf("unexpected ArtifactoryVersion: %q", got.System.ArtifactoryVersion)
	}
	if len(got.Users) != 1 || got.Users[0].Username != "alice" {
		t.Errorf("unexpected Users: %+v", got.Users)
	}
	if len(got.Warnings) != 1 {
		t.Errorf("unexpected Warnings: %+v", got.Warnings)
	}
}

func TestReadJSON_InvalidInput(t *testing.T) {
	if _, err := ReadJSON(bytes.NewReader([]byte("not json"))); err == nil {
		t.Fatal("expected error decoding invalid JSON, got nil")
	}
}
