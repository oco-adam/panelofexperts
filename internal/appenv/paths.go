package appenv

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	EnvHome            = "POE_HOME"
	EnvManifestURL     = "POE_RELEASE_MANIFEST_URL"
	DefaultManifestURL = "https://github.com/oco-adam/panelofexperts/releases/latest/download/release-manifest.json"
	ReceiptFileName    = "install-receipt.json"
)

func HomeDir(getenv func(string) string) (string, error) {
	if getenv != nil {
		if override := strings.TrimSpace(getenv(EnvHome)); override != "" {
			return filepath.Abs(override)
		}
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "poe"), nil
}

func ReceiptPath(getenv func(string) string) (string, error) {
	homeDir, err := HomeDir(getenv)
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ReceiptFileName), nil
}

func ManifestURL(getenv func(string) string) string {
	if getenv != nil {
		if override := strings.TrimSpace(getenv(EnvManifestURL)); override != "" {
			return override
		}
	}
	return DefaultManifestURL
}

func WorkspaceOutputRoot(cwd string) string {
	return filepath.Join(cwd, ".panel-of-experts", "runs")
}
