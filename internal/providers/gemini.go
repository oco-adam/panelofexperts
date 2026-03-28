package providers

import (
	"context"
	"os"

	"panelofexperts/internal/model"
)

type GeminiProvider struct {
	Binary string
}

func NewGeminiProvider(binary string) *GeminiProvider {
	if binary == "" {
		binary = "gemini"
	}
	return &GeminiProvider{Binary: binary}
}

func (p *GeminiProvider) ID() model.ProviderID {
	return model.ProviderGemini
}

func (p *GeminiProvider) Detect(_ context.Context) model.Capability {
	path, err := detectBinary(p.Binary)
	if err != nil {
		return model.Capability{
			Provider:    model.ProviderGemini,
			DisplayName: model.ProviderDisplayName(model.ProviderGemini),
			Summary:     "Gemini CLI not found",
			Error:       err.Error(),
		}
	}
	authenticated := hasAnyFile(
		authFile(".gemini", "oauth_creds.json"),
		authFile(".gemini", "google_accounts.json"),
	)
	summary := "Binary found"
	if authenticated {
		summary = "Binary and auth state found"
	}
	return model.Capability{
		Provider:      model.ProviderGemini,
		DisplayName:   model.ProviderDisplayName(model.ProviderGemini),
		BinaryPath:    path,
		Available:     true,
		Authenticated: authenticated,
		Summary:       summary,
	}
}

func (p *GeminiProvider) Run(ctx context.Context, request model.Request, progress chan<- model.ProgressEvent) (model.Result, error) {
	execCWD := request.CWD
	args := []string{
		"--model", "gemini-2.5-flash",
		"--prompt", request.Prompt,
		"--output-format", "json",
		"--approval-mode", "plan",
	}
	if request.Metadata["workspace_access"] == "none" {
		execCWD = os.TempDir()
	} else {
		args = append(args, "--include-directories", request.CWD)
	}
	return executeCommand(
		ctx,
		model.ProviderGemini,
		p.Binary,
		args,
		"",
		execCWD,
		request,
		progress,
		nil,
	)
}
