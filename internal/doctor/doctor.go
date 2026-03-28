package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"panelofexperts/internal/appenv"
	"panelofexperts/internal/buildinfo"
	"panelofexperts/internal/model"
	"panelofexperts/internal/orchestrator"
	"panelofexperts/internal/release"
)

type Options struct {
	CWD        string
	OutputRoot string
	Getenv     func(string) string
	HTTPClient *http.Client
}

type Report struct {
	Build        buildinfo.Info
	CWD          string
	OutputRoot   string
	AppHome      string
	ReceiptPath  string
	Receipt      *release.InstallReceipt
	Capabilities []model.Capability
	Update       UpdateStatus
}

type UpdateStatus struct {
	ManifestURL   string
	LatestVersion string
	Status        string
	Message       string
}

func Gather(ctx context.Context, engine *orchestrator.Engine, options Options) (Report, error) {
	report := Report{
		Build:      buildinfo.Current(),
		CWD:        options.CWD,
		OutputRoot: options.OutputRoot,
		Update: UpdateStatus{
			ManifestURL: appenv.ManifestURL(options.Getenv),
			Status:      "not_checked",
		},
	}

	appHome, err := appenv.HomeDir(options.Getenv)
	if err != nil {
		return report, err
	}
	report.AppHome = appHome

	receiptPath, err := appenv.ReceiptPath(options.Getenv)
	if err != nil {
		return report, err
	}
	report.ReceiptPath = receiptPath
	if receipt, err := release.ReadInstallReceipt(receiptPath); err == nil {
		report.Receipt = receipt
	}

	if engine != nil {
		detected := engine.DetectCapabilities(ctx)
		report.Capabilities = make([]model.Capability, 0, len(model.AllProviders()))
		for _, providerID := range model.AllProviders() {
			report.Capabilities = append(report.Capabilities, detected[providerID])
		}
	}

	report.Update = checkForUpdate(ctx, report.Build, report.Receipt, report.Update.ManifestURL, options.HTTPClient)
	return report, nil
}

func RenderText(report Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", report.Build.Summary())
	fmt.Fprintf(&b, "build commit: %s\n", report.Build.Commit)
	fmt.Fprintf(&b, "build date: %s\n", report.Build.Date)
	fmt.Fprintf(&b, "build source: %s\n", report.Build.BuiltBy)
	fmt.Fprintf(&b, "cwd: %s\n", report.CWD)
	fmt.Fprintf(&b, "output root: %s\n", report.OutputRoot)
	fmt.Fprintf(&b, "app home: %s\n", report.AppHome)
	fmt.Fprintf(&b, "receipt path: %s\n", report.ReceiptPath)
	if report.Receipt == nil {
		b.WriteString("install receipt: not found\n")
	} else {
		fmt.Fprintf(&b, "installed version: %s\n", emptyOrFallback(report.Receipt.Version, "unknown"))
		fmt.Fprintf(&b, "install channel: %s\n", emptyOrFallback(report.Receipt.Channel, "unknown"))
		fmt.Fprintf(&b, "install path: %s\n", emptyOrFallback(report.Receipt.InstallPath, "unknown"))
		if report.Receipt.SourceURL != "" {
			fmt.Fprintf(&b, "install source: %s\n", report.Receipt.SourceURL)
		}
	}

	b.WriteString("\nProviders\n")
	for _, capability := range report.Capabilities {
		status := "missing"
		switch {
		case capability.Available && capability.Authenticated:
			status = "ready"
		case capability.Available:
			status = "available"
		}
		fmt.Fprintf(&b, "- %s: %s\n", capability.DisplayName, status)
		if capability.BinaryPath != "" {
			fmt.Fprintf(&b, "  binary: %s\n", capability.BinaryPath)
		}
		if capability.Summary != "" {
			fmt.Fprintf(&b, "  summary: %s\n", capability.Summary)
		}
		if capability.Error != "" {
			fmt.Fprintf(&b, "  error: %s\n", capability.Error)
		}
	}

	b.WriteString("\nUpdates\n")
	fmt.Fprintf(&b, "- manifest: %s\n", emptyOrFallback(report.Update.ManifestURL, "not configured"))
	fmt.Fprintf(&b, "- status: %s\n", emptyOrFallback(report.Update.Status, "unknown"))
	if report.Update.LatestVersion != "" {
		fmt.Fprintf(&b, "- latest release: %s\n", report.Update.LatestVersion)
	}
	if report.Update.Message != "" {
		fmt.Fprintf(&b, "- action: %s\n", report.Update.Message)
	}

	return strings.TrimSpace(b.String()) + "\n"
}

func checkForUpdate(ctx context.Context, build buildinfo.Info, receipt *release.InstallReceipt, manifestURL string, client *http.Client) UpdateStatus {
	status := UpdateStatus{
		ManifestURL: manifestURL,
		Status:      "skipped",
		Message:     "Using a development build or missing release metadata.",
	}
	if strings.TrimSpace(manifestURL) == "" {
		status.Message = "No manifest URL configured."
		return status
	}
	if !semver.IsValid(build.Version) {
		return status
	}

	httpClient := client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 2 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return UpdateStatus{ManifestURL: manifestURL, Status: "error", Message: err.Error()}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return UpdateStatus{ManifestURL: manifestURL, Status: "error", Message: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return UpdateStatus{
			ManifestURL: manifestURL,
			Status:      "error",
			Message:     fmt.Sprintf("manifest returned %s", resp.Status),
		}
	}
	var manifest release.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return UpdateStatus{ManifestURL: manifestURL, Status: "error", Message: err.Error()}
	}
	latest := strings.TrimSpace(manifest.Version)
	if !semver.IsValid(latest) {
		return UpdateStatus{ManifestURL: manifestURL, Status: "error", Message: "manifest version is not valid semver"}
	}
	if semver.Compare(latest, build.Version) > 0 {
		message := "Rerun the installer to update to the latest release."
		if receipt == nil || receipt.Channel == "" {
			message = "Install the latest release if you want the current published version."
		}
		return UpdateStatus{
			ManifestURL:   manifestURL,
			LatestVersion: latest,
			Status:        "update_available",
			Message:       message,
		}
	}
	return UpdateStatus{
		ManifestURL:   manifestURL,
		LatestVersion: latest,
		Status:        "up_to_date",
		Message:       "No update action required.",
	}
}

func emptyOrFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func ReceiptExists(getenv func(string) string) bool {
	receiptPath, err := appenv.ReceiptPath(getenv)
	if err != nil {
		return false
	}
	_, err = os.Stat(receiptPath)
	return err == nil
}
