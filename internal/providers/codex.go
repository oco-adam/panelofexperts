package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"panelofexperts/internal/model"
)

type CodexProvider struct {
	Binary string
}

func NewCodexProvider(binary string) *CodexProvider {
	if binary == "" {
		binary = "codex"
	}
	return &CodexProvider{Binary: binary}
}

func (p *CodexProvider) ID() model.ProviderID {
	return model.ProviderCodex
}

func (p *CodexProvider) Detect(_ context.Context) model.Capability {
	path, err := detectBinary(p.Binary)
	if err != nil {
		return model.Capability{
			Provider:    model.ProviderCodex,
			DisplayName: model.ProviderDisplayName(model.ProviderCodex),
			Summary:     "Codex CLI not found",
			Error:       err.Error(),
		}
	}
	authPath := authFile(".codex", "auth.json")
	authenticated := hasAnyFile(authPath)
	summary := "Binary found"
	if authenticated {
		summary = "Binary and auth state found"
	}
	return model.Capability{
		Provider:      model.ProviderCodex,
		DisplayName:   model.ProviderDisplayName(model.ProviderCodex),
		BinaryPath:    path,
		Available:     true,
		Authenticated: authenticated,
		Summary:       summary,
	}
}

func (p *CodexProvider) Run(ctx context.Context, request model.Request, progress chan<- model.ProgressEvent) (model.Result, error) {
	schemaFile, err := os.CreateTemp("", "panelofexperts-codex-schema-*.json")
	if err != nil {
		return model.Result{}, err
	}
	defer os.Remove(schemaFile.Name())
	if _, err := schemaFile.WriteString(request.JSONSchema); err != nil {
		return model.Result{}, err
	}
	if err := schemaFile.Close(); err != nil {
		return model.Result{}, err
	}

	outputFile, err := os.CreateTemp("", "panelofexperts-codex-output-*.json")
	if err != nil {
		return model.Result{}, err
	}
	outputFile.Close()
	defer os.Remove(outputFile.Name())

	args := []string{
		"exec",
		"-C", request.CWD,
		"-s", "read-only",
		"--ephemeral",
		"--json",
		"--output-schema", schemaFile.Name(),
		"--output-last-message", outputFile.Name(),
		"-",
	}
	if !insideGitRepo(ctx, request.CWD) {
		args = append(args[:len(args)-1], append([]string{"--skip-git-repo-check"}, args[len(args)-1:]...)...)
	}

	result, err := executeCommand(
		ctx,
		model.ProviderCodex,
		p.Binary,
		args,
		request.Prompt,
		request.CWD,
		request,
		progress,
		parseCodexLine,
	)
	if err != nil {
		return result, err
	}
	data, readErr := os.ReadFile(outputFile.Name())
	if readErr == nil && strings.TrimSpace(string(data)) != "" {
		result.Stdout = string(data)
	}
	result.Metadata["schema_file"] = filepath.Base(schemaFile.Name())
	result.Metadata["output_file"] = filepath.Base(outputFile.Name())
	return result, nil
}

func parseCodexLine(line string, progress chan<- model.ProgressEvent, request model.Request) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		return
	}

	step := asString(payload["type"])
	if step == "" {
		step = asString(payload["event"])
	}
	summary := asString(payload["message"])
	if summary == "" {
		summary = asString(payload["summary"])
	}
	if summary == "" {
		summary = fmt.Sprintf("Codex event: %s", step)
	}
	emitProgress(progress, model.ProgressEvent{
		Timestamp: time.Now().UTC(),
		RunID:     request.RunID,
		Round:     request.Round,
		AgentID:   request.AgentID,
		Role:      request.Role,
		Provider:  model.ProviderCodex,
		State:     model.AgentStateRunning,
		Step:      step,
		Summary:   summarizeLine(summary),
	})
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}
