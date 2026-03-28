package release

import (
	"encoding/json"
	"os"
	"strings"
)

type Manifest struct {
	Version      string          `json:"version"`
	PublishedAt  string          `json:"published_at,omitempty"`
	NotesURL     string          `json:"notes_url,omitempty"`
	ChecksumsURL string          `json:"checksums_url,omitempty"`
	Assets       []ManifestAsset `json:"assets"`
}

type ManifestAsset struct {
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Archive string `json:"archive"`
}

type InstallReceipt struct {
	Version     string `json:"version"`
	Channel     string `json:"channel"`
	InstalledAt string `json:"installed_at"`
	InstallPath string `json:"install_path"`
	SourceURL   string `json:"source_url,omitempty"`
	Repository  string `json:"repository,omitempty"`
}

func ReadInstallReceipt(path string) (*InstallReceipt, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var receipt InstallReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return nil, err
	}
	receipt.Version = strings.TrimSpace(receipt.Version)
	receipt.Channel = strings.TrimSpace(receipt.Channel)
	receipt.InstallPath = strings.TrimSpace(receipt.InstallPath)
	receipt.SourceURL = strings.TrimSpace(receipt.SourceURL)
	receipt.Repository = strings.TrimSpace(receipt.Repository)
	return &receipt, nil
}
