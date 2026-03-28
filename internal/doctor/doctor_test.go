package doctor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"panelofexperts/internal/buildinfo"
	"panelofexperts/internal/release"
)

func TestCheckForUpdateReportsAvailableRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"v9.9.9","assets":[]}`))
	}))
	defer server.Close()

	status := checkForUpdate(context.Background(), buildinfo.Info{Version: "v1.0.0"}, &release.InstallReceipt{Channel: "direct"}, server.URL, server.Client())
	if status.Status != "update_available" {
		t.Fatalf("expected update_available, got %q", status.Status)
	}
	if status.LatestVersion != "v9.9.9" {
		t.Fatalf("expected latest version to be reported, got %q", status.LatestVersion)
	}
	if !strings.Contains(status.Message, "Rerun the installer") {
		t.Fatalf("expected install guidance, got %q", status.Message)
	}
}

func TestRenderTextIncludesReceiptSummary(t *testing.T) {
	output := RenderText(Report{
		Build:       buildinfo.Info{Version: "v1.0.0", Commit: "abc123", Date: "2026-03-28T12:00:00Z", BuiltBy: "test"},
		CWD:         "/workspace",
		OutputRoot:  "/workspace/.panel-of-experts/runs",
		AppHome:     "/config/poe",
		ReceiptPath: "/config/poe/install-receipt.json",
		Receipt: &release.InstallReceipt{
			Version:     "v1.0.0",
			Channel:     "direct",
			InstallPath: "/usr/local/bin/poe",
		},
		Update: UpdateStatus{
			ManifestURL:   "https://example.test/release-manifest.json",
			Status:        "up_to_date",
			LatestVersion: "v1.0.0",
			Message:       "No update action required.",
		},
	})

	for _, expected := range []string{
		"installed version: v1.0.0",
		"install channel: direct",
		"install path: /usr/local/bin/poe",
		"latest release: v1.0.0",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in doctor output, got:\n%s", expected, output)
		}
	}
}
