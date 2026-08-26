package discover

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetSystemInfo_ArtifactoryRequiredOthersOptional(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/artifactory/api/system/version":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"version": "7.165.0", "revision": "86500900", "addons": []string{"docker"},
			})
		case "/xray/api/v1/system/version":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"xray_version": "3.152.6", "xray_revision": "abc"})
		case "/access/api/v1/system/version":
			w.WriteHeader(http.StatusNotFound)
		case "/distribution/api/v1/system/version":
			w.WriteHeader(http.StatusNotFound)
		case "/artifactory/api/storageinfo":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"binariesSummary":  map[string]string{"binariesCount": "100", "binariesSize": "1 GB"},
				"fileStoreSummary": map[string]string{"storageType": "s3"},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	info, warnings, err := GetSystemInfo(testClient(t, srv))
	if err != nil {
		t.Fatalf("GetSystemInfo returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings for clean 404s, got %v", warnings)
	}
	if info.ArtifactoryVersion != "7.165.0" {
		t.Errorf("unexpected ArtifactoryVersion: %q", info.ArtifactoryVersion)
	}
	if !info.XrayAvailable || info.XrayVersion != "3.152.6" {
		t.Errorf("unexpected Xray info: available=%v version=%q", info.XrayAvailable, info.XrayVersion)
	}
	if info.AccessAvailable || info.DistributionAvailable {
		t.Errorf("expected Access/Distribution unavailable on 404, got access=%v distribution=%v", info.AccessAvailable, info.DistributionAvailable)
	}
	if info.StorageType != "s3" || info.BinariesCount != "100" {
		t.Errorf("unexpected storage summary: %+v", info)
	}
}

func TestGetSystemInfo_FailsFastWhenArtifactoryVersionErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	if _, _, err := GetSystemInfo(testClient(t, srv)); err == nil {
		t.Fatal("expected error when the required Artifactory version fetch fails")
	}
}
