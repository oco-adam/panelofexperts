package providers

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"panelofexperts/internal/model"
)

type ClaudeProvider struct {
	Binary string
}

func NewClaudeProvider(binary string) *ClaudeProvider {
	if binary == "" {
		binary = "claude"
	}
	return &ClaudeProvider{Binary: binary}
}

func (p *ClaudeProvider) ID() model.ProviderID {
	return model.ProviderClaude
}

func (p *ClaudeProvider) Detect(ctx context.Context) model.Capability {
	path, err := detectBinary(p.Binary)
	if err != nil {
		return model.Capability{
			Provider:    model.ProviderClaude,
			DisplayName: model.ProviderDisplayName(model.ProviderClaude),
			Summary:     "Claude CLI not found",
			Error:       err.Error(),
		}
	}

	authCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(authCtx, p.Binary, "auth", "status")
	out, authErr := cmd.CombinedOutput()
	authenticated := authErr == nil
	summary := "Binary found"
	if authenticated {
		summary = "Binary and auth status found"
	} else if strings.TrimSpace(string(out)) != "" {
		summary = summarizeLine(string(out))
	}
	return model.Capability{
		Provider:      model.ProviderClaude,
		DisplayName:   model.ProviderDisplayName(model.ProviderClaude),
		BinaryPath:    path,
		Available:     true,
		Authenticated: authenticated,
		Summary:       summary,
	}
}

func (p *ClaudeProvider) Run(ctx context.Context, request model.Request, progress chan<- model.ProgressEvent) (model.Result, error) {
	args := []string{
		"-p",
		"--bare",
		"--permission-mode", "plan",
		"--no-session-persistence",
		"--add-dir", request.CWD,
		"--json-schema", request.JSONSchema,
		request.Prompt,
	}
	return executeCommand(
		ctx,
		model.ProviderClaude,
		p.Binary,
		args,
		"",
		request.CWD,
		request,
		progress,
		nil,
	)
}
