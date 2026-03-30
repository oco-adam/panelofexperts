package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"panelofexperts/internal/appenv"
	"panelofexperts/internal/model"
	"panelofexperts/internal/orchestrator"
)

func TestRunVersionCommand(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
	}

	exitCode := app.Run(context.Background(), []string{"version"})
	if exitCode != 0 {
		t.Fatalf("expected success, got %d", exitCode)
	}
	output := stdout.String()
	for _, expected := range []string{"poe ", "commit:", "date:", "built by:"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in version output, got:\n%s", expected, output)
		}
	}
}

func TestRunDoctorSucceedsWithoutConfiguredProviders(t *testing.T) {
	tempDir := t.TempDir()
	var stdout bytes.Buffer
	app := &App{
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
		Getwd: func() (string, error) {
			return tempDir, nil
		},
		Getenv: func(key string) string {
			if key == "POE_HOME" {
				return t.TempDir()
			}
			return ""
		},
	}

	exitCode := app.Run(context.Background(), []string{"doctor"})
	if exitCode != 0 {
		t.Fatalf("expected doctor to exit 0, got %d", exitCode)
	}
	output := stdout.String()
	for _, expected := range []string{"Providers", "Codex CLI", "Claude CLI", "Gemini CLI"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in doctor output, got:\n%s", expected, output)
		}
	}
}

func TestRunInteractiveLaunchesUIWithResolvedPaths(t *testing.T) {
	tempDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	var launched bool
	var launchedCWD string
	var launchedOutputRoot string
	var launchedDebug bool

	app := &App{
		Stdout: &stdout,
		Stderr: &stderr,
		Getwd: func() (string, error) {
			return tempDir, nil
		},
		LaunchUI: func(_ *orchestrator.Engine, cwd, outputRoot string, debug bool) error {
			launched = true
			launchedCWD = cwd
			launchedOutputRoot = outputRoot
			launchedDebug = debug
			return nil
		},
	}

	exitCode := app.Run(context.Background(), []string{"--debug"})
	if exitCode != 0 {
		t.Fatalf("expected success, got %d", exitCode)
	}
	if !launched {
		t.Fatal("expected interactive launch to run")
	}
	if launchedCWD != tempDir {
		t.Fatalf("expected cwd %q, got %q", tempDir, launchedCWD)
	}
	expectedOutputRoot := appenv.WorkspaceOutputRoot(tempDir)
	if launchedOutputRoot != expectedOutputRoot {
		t.Fatalf("expected output root %q, got %q", expectedOutputRoot, launchedOutputRoot)
	}
	if !launchedDebug {
		t.Fatal("expected debug mode to be forwarded")
	}
}

func TestRunInteractiveReturnsErrorWhenGetwdFails(t *testing.T) {
	app := &App{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		Getwd: func() (string, error) {
			return "", errors.New("boom")
		},
	}
	if exitCode := app.Run(context.Background(), nil); exitCode != 1 {
		t.Fatalf("expected exit 1, got %d", exitCode)
	}
}

func TestRunRunsCommandListsSavedRuns(t *testing.T) {
	tempDir := t.TempDir()
	outputRoot := appenv.WorkspaceOutputRoot(tempDir)
	run := model.NewRunState(
		"20260329-224313-example",
		tempDir,
		filepath.Join(outputRoot, "20260329-224313-example"),
		5,
		model.MergeStrategyTogether,
		model.AgentConfig{ID: "manager", Name: "Manager (Codex CLI)", Role: model.RoleManager, Provider: model.ProviderCodex},
		[]model.AgentConfig{{ID: "expert-1", Name: "Expert 1 (Claude CLI)", Role: model.RoleExpert, Provider: model.ProviderClaude, Lens: model.LensArchitecture}},
	)
	run.ProjectTitle = "Saved run"
	run.Status = model.RunStatusFailed
	run.CurrentPhase = "deliverable_draft_failed"
	run.Brief.TaskKind = model.TaskKindDocument
	run.FinalProposal = &model.Proposal{Title: "Saved proposal"}
	run.UpdatedAt = time.Date(2026, 3, 30, 9, 30, 0, 0, time.UTC)

	store, err := orchestrator.NewStore(run.OutputDir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := store.SaveState(run); err != nil {
		t.Fatalf("save state: %v", err)
	}

	var stdout bytes.Buffer
	app := &App{
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
		Getwd: func() (string, error) {
			return tempDir, nil
		},
	}

	if exitCode := app.Run(context.Background(), []string{"runs"}); exitCode != 0 {
		t.Fatalf("expected success, got %d", exitCode)
	}
	output := stdout.String()
	for _, expected := range []string{"20260329-224313-example", "retry, rerun", "Saved run"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in runs output, got:\n%s", expected, output)
		}
	}
}

func TestRunRunsCommandUsesCWDToResolveDefaultOutputRoot(t *testing.T) {
	currentDir := t.TempDir()
	otherRepo := t.TempDir()
	outputRoot := appenv.WorkspaceOutputRoot(otherRepo)
	run := model.NewRunState(
		"20260330-101500-otherrepo",
		otherRepo,
		filepath.Join(outputRoot, "20260330-101500-otherrepo"),
		5,
		model.MergeStrategyTogether,
		model.AgentConfig{ID: "manager", Name: "Manager (Codex CLI)", Role: model.RoleManager, Provider: model.ProviderCodex},
		[]model.AgentConfig{{ID: "expert-1", Name: "Expert 1 (Claude CLI)", Role: model.RoleExpert, Provider: model.ProviderClaude, Lens: model.LensArchitecture}},
	)
	run.ProjectTitle = "Other repo run"
	run.UpdatedAt = time.Date(2026, 3, 30, 10, 15, 0, 0, time.UTC)

	store, err := orchestrator.NewStore(run.OutputDir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := store.SaveState(run); err != nil {
		t.Fatalf("save state: %v", err)
	}

	var stdout bytes.Buffer
	app := &App{
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
		Getwd: func() (string, error) {
			return currentDir, nil
		},
	}

	if exitCode := app.Run(context.Background(), []string{"runs", "--cwd", otherRepo}); exitCode != 0 {
		t.Fatalf("expected success, got %d", exitCode)
	}
	if !strings.Contains(stdout.String(), "Other repo run") {
		t.Fatalf("expected runs output to resolve the other repo output root, got:\n%s", stdout.String())
	}
}

func TestRunRetryCommandResumesSavedDeliverableRun(t *testing.T) {
	tempDir := t.TempDir()
	outputRoot := appenv.WorkspaceOutputRoot(tempDir)
	targetFile := filepath.Join(tempDir, "DESIGN.md")

	var sawTimeout time.Duration
	engine := orchestrator.NewEngine(orchestratorFakeProvider{
		id: model.ProviderCodex,
		run: func(request model.Request) (string, error) {
			if request.OutputKind != "deliverable" {
				t.Fatalf("expected deliverable request, got %q", request.OutputKind)
			}
			sawTimeout = request.Timeout
			return `{"path":"` + targetFile + `","markdown":"# Resumed deliverable"}`, nil
		},
	})

	run, err := engine.NewRun(orchestrator.NewRunOptions{
		CWD:             tempDir,
		OutputRoot:      outputRoot,
		ManagerProvider: model.ProviderCodex,
		ExpertProviders: []model.ProviderID{model.ProviderCodex, model.ProviderCodex},
	})
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	run.CurrentRound = 1
	run.Status = model.RunStatusFailed
	run.CurrentPhase = "deliverable_draft_failed"
	run.StopReason = model.StopReasonManagerFailed
	run.FailureSummary = "codex timed out after 1h0m0s"
	run.Brief = model.Brief{
		ProjectTitle:   "Retry app test",
		IntentSummary:  "Retry the document phase",
		TaskKind:       model.TaskKindDocument,
		TargetFilePath: targetFile,
		Goals:          []string{"Write the doc"},
		ReadyToStart:   true,
	}
	run.FinalProposal = &model.Proposal{
		Title:           "Final proposal",
		Context:         "Ready for document drafting.",
		Goals:           []string{"Write the doc"},
		RecommendedPlan: []model.PlanItem{{Title: "Write", Details: "Write the doc."}},
		DeliverablePath: targetFile,
	}

	store, err := orchestrator.NewStore(run.OutputDir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := store.SaveState(run); err != nil {
		t.Fatalf("save state: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := &App{
		Engine: engine,
		Stdout: &stdout,
		Stderr: &stderr,
		Getwd: func() (string, error) {
			return tempDir, nil
		},
	}

	if exitCode := app.Run(context.Background(), []string{"retry", "--run", run.ID, "--deliverable-timeout", "2h"}); exitCode != 0 {
		t.Fatalf("expected success, got %d, stderr:\n%s", exitCode, stderr.String())
	}
	if sawTimeout != 2*time.Hour {
		t.Fatalf("expected retry timeout override of 2h, got %s", sawTimeout)
	}
	if _, err := os.Stat(targetFile); err != nil {
		t.Fatalf("expected resumed deliverable to be written: %v", err)
	}
	if !strings.Contains(stdout.String(), "Retry completed") {
		t.Fatalf("expected success output, got:\n%s", stdout.String())
	}
}

type orchestratorFakeProvider struct {
	id  model.ProviderID
	run func(request model.Request) (string, error)
}

func (f orchestratorFakeProvider) ID() model.ProviderID { return f.id }

func (f orchestratorFakeProvider) Detect(context.Context) model.Capability {
	return model.Capability{
		Provider:      f.id,
		DisplayName:   model.ProviderDisplayName(f.id),
		Available:     true,
		Authenticated: true,
		Summary:       "ready",
	}
}

func (f orchestratorFakeProvider) Run(_ context.Context, request model.Request, progress chan<- model.ProgressEvent) (model.Result, error) {
	raw, err := f.run(request)
	return model.Result{
		Provider:    f.id,
		StartedAt:   time.Now().UTC(),
		CompletedAt: time.Now().UTC(),
		Stdout:      raw,
		ExitCode:    0,
	}, err
}
