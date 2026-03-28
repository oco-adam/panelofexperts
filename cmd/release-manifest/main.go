package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"panelofexperts/internal/release"
)

var archivePattern = regexp.MustCompile(`^poe_(.+)_(darwin|linux|windows)_([^./]+)\.(tar\.gz|zip)$`)

func main() {
	var distDir string
	var version string
	var repo string
	var output string
	var publishedAt string

	flag.StringVar(&distDir, "dist", "dist", "directory containing goreleaser artifacts")
	flag.StringVar(&version, "version", "", "release version such as v1.2.3")
	flag.StringVar(&repo, "repo", "", "GitHub repository in owner/name form")
	flag.StringVar(&output, "output", "", "output path for release-manifest.json")
	flag.StringVar(&publishedAt, "published-at", "", "release timestamp in RFC3339")
	flag.Parse()

	if strings.TrimSpace(version) == "" {
		fail("missing -version")
	}
	if strings.TrimSpace(repo) == "" {
		fail("missing -repo")
	}
	if output == "" {
		output = filepath.Join(distDir, "release-manifest.json")
	}
	if publishedAt == "" {
		publishedAt = time.Now().UTC().Format(time.RFC3339)
	}

	checksums, err := readChecksums(filepath.Join(distDir, "checksums.txt"))
	if err != nil {
		fail(err.Error())
	}

	entries, err := os.ReadDir(distDir)
	if err != nil {
		fail(err.Error())
	}

	baseURL := fmt.Sprintf("https://github.com/%s/releases/download/%s", repo, version)
	manifest := release.Manifest{
		Version:      version,
		PublishedAt:  publishedAt,
		NotesURL:     fmt.Sprintf("https://github.com/%s/releases/tag/%s", repo, version),
		ChecksumsURL: baseURL + "/checksums.txt",
		Assets:       []release.ManifestAsset{},
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		matches := archivePattern.FindStringSubmatch(name)
		if len(matches) != 5 {
			continue
		}
		if matches[1] != version {
			continue
		}
		manifest.Assets = append(manifest.Assets, release.ManifestAsset{
			OS:      matches[2],
			Arch:    matches[3],
			Name:    name,
			URL:     baseURL + "/" + name,
			SHA256:  checksums[name],
			Archive: matches[4],
		})
	}

	sort.Slice(manifest.Assets, func(i, j int) bool {
		left := manifest.Assets[i]
		right := manifest.Assets[j]
		if left.OS == right.OS {
			return left.Arch < right.Arch
		}
		return left.OS < right.OS
	})

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		fail(err.Error())
	}
	if err := os.WriteFile(output, append(data, '\n'), 0o644); err != nil {
		fail(err.Error())
	}
}

func readChecksums(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	checksums := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			continue
		}
		checksums[fields[1]] = fields[0]
	}
	return checksums, nil
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
