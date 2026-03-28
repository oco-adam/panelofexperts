package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"panelofexperts/internal/model"
)

type fakeProvider struct {
	id  model.ProviderID
	run func(request model.Request) (string, error)
}

func (f fakeProvider) ID() model.ProviderID { return f.id }

func (f fakeProvider) Detect(context.Context) model.Capability {
	return model.Capability{
		Provider:      f.id,
		DisplayName:   model.ProviderDisplayName(f.id),
		Available:     true,
		Authenticated: true,
		Summary:       "ready",
	}
}

func (f fakeProvider) Run(_ context.Context, request model.Request, progress chan<- model.ProgressEvent) (model.Result, error) {
	if progress != nil {
		progress <- model.ProgressEvent{
			Timestamp: time.Now().UTC(),
			RunID:     request.RunID,
			Round:     request.Round,
			AgentID:   request.AgentID,
			Role:      request.Role,
			Provider:  f.id,
			State:     model.AgentStateRunning,
			Step:      request.OutputKind,
			Summary:   "fake provider running",
		}
	}
	raw, err := f.run(request)
	return model.Result{
		Provider:    f.id,
		StartedAt:   time.Now().UTC(),
		CompletedAt: time.Now().UTC(),
		Stdout:      raw,
		ExitCode:    0,
	}, err
}

func TestEngineUpdateBriefAndRunDiscussion(t *testing.T) {
	tempDir := t.TempDir()

	handler := func(request model.Request) (string, error) {
		switch request.OutputKind {
		case "brief":
			return mustMarshal(t, model.Brief{
				ProjectTitle:   "Panel Test Project",
				IntentSummary:  "Build the planning workflow.",
				TaskKind:       model.TaskKindPlan,
				TargetFilePath: "",
				Goals:          []string{"Plan the app"},
				Constraints:    []string{"Read-only"},
				ReadyToStart:   true,
				OpenQuestions:  []string{"None"},
				ManagerNotes:   "Ready for expert review.",
			}), nil
		case "review":
			return mustMarshal(t, model.ExpertReview{
				Lens:            request.Lens,
				Summary:         "Looks good.",
				Strengths:       []string{"Clear structure"},
				Concerns:        []string{"Minor sequencing tweak"},
				Recommendations: []string{"Keep status visibility prominent"},
				BlockingRisks:   []string{},
				RequiresChanges: request.Lens == model.LensArchitecture,
			}), nil
		case "proposal":
			switch request.Version {
			case 1:
				return mustMarshal(t, model.Proposal{
					Title:               "Initial proposal",
					Context:             "Initial context.",
					Goals:               []string{"Plan the app"},
					Constraints:         []string{"Read-only"},
					RecommendedPlan:     []model.PlanItem{{Title: "Draft", Details: "Create the first proposal."}},
					Risks:               []string{"Need convergence logic"},
					OpenQuestions:       []string{"How much status to show?"},
					ConsensusNotes:      []string{"Initial draft only"},
					DeliverablePath:     "",
					DeliverableMarkdown: "",
					Converged:           false,
					ChangeSummary:       "Initial manager draft.",
				}), nil
			case 2:
				return mustMarshal(t, model.Proposal{
					Title:               "Merged proposal",
					Context:             "Context after architecture review.",
					Goals:               []string{"Plan the app"},
					Constraints:         []string{"Read-only"},
					RecommendedPlan:     []model.PlanItem{{Title: "Refine", Details: "Add live status board."}},
					Risks:               []string{"Need deterministic convergence"},
					OpenQuestions:       []string{},
					ConsensusNotes:      []string{"Architecture feedback merged"},
					DeliverablePath:     "",
					DeliverableMarkdown: "",
					Converged:           false,
					ChangeSummary:       "Merged architecture feedback.",
				}), nil
			default:
				return mustMarshal(t, model.Proposal{
					Title:               "Final proposal",
					Context:             "Context after all reviews.",
					Goals:               []string{"Plan the app"},
					Constraints:         []string{"Read-only"},
					RecommendedPlan:     []model.PlanItem{{Title: "Ship", Details: "Implement the approved plan."}},
					Risks:               []string{"No major blockers"},
					OpenQuestions:       []string{},
					ConsensusNotes:      []string{"Panel converged"},
					DeliverablePath:     "",
					DeliverableMarkdown: "",
					Converged:           true,
					ChangeSummary:       "Execution feedback merged; proposal converged.",
				}), nil
			}
		default:
			t.Fatalf("unexpected output kind %q", request.OutputKind)
			return "", nil
		}
	}

	engine := NewEngine(
		fakeProvider{id: model.ProviderCodex, run: handler},
		fakeProvider{id: model.ProviderClaude, run: handler},
		fakeProvider{id: model.ProviderGemini, run: handler},
	)

	run, err := engine.NewRun(NewRunOptions{
		CWD:             tempDir,
		OutputRoot:      filepath.Join(tempDir, "runs"),
		MaxRounds:       10,
		ManagerProvider: model.ProviderCodex,
		ExpertProviders: []model.ProviderID{model.ProviderClaude, model.ProviderGemini},
	})
	if err != nil {
		t.Fatalf("new run: %v", err)
	}

	run, err = engine.UpdateBrief(context.Background(), run, "Build the app", nil)
	if err != nil {
		t.Fatalf("update brief: %v", err)
	}
	if run.ProjectTitle != "Panel Test Project" {
		t.Fatalf("expected project title to update, got %q", run.ProjectTitle)
	}

	run, err = engine.RunDiscussion(context.Background(), run, nil)
	if err != nil {
		t.Fatalf("run discussion: %v", err)
	}
	if run.FinalProposal == nil {
		t.Fatal("expected final proposal to be present")
	}
	if run.StopReason != model.StopReasonConverged {
		t.Fatalf("expected converged stop reason, got %q", run.StopReason)
	}
	if run.WaitingSummary != "Discussion converged" {
		t.Fatalf("expected converged waiting summary, got %q", run.WaitingSummary)
	}
	if len(run.Rounds) != 1 {
		t.Fatalf("expected one completed round, got %d", len(run.Rounds))
	}

	for _, rel := range []string{
		"brief.json",
		"brief.md",
		"proposal-v001.json",
		"proposal-v002.json",
		"proposal-v003.json",
		"reviews/round-1/expert-1.json",
		"reviews/round-1/expert-2.json",
		"final.md",
		"state.json",
	} {
		if _, err := os.Stat(filepath.Join(run.OutputDir, rel)); err != nil {
			t.Fatalf("expected artifact %s: %v", rel, err)
		}
	}

	finalData, err := os.ReadFile(filepath.Join(run.OutputDir, "final.md"))
	if err != nil {
		t.Fatalf("read final markdown: %v", err)
	}
	if !strings.Contains(string(finalData), "# Final proposal") {
		t.Fatalf("expected final markdown to contain final proposal title, got:\n%s", string(finalData))
	}
}

func TestEngineWritesDocumentDeliverableToTargetFile(t *testing.T) {
	tempDir := t.TempDir()
	targetFile := filepath.Join(tempDir, "DESIGN.md")

	handler := func(request model.Request) (string, error) {
		switch request.OutputKind {
		case "brief":
			return mustMarshal(t, model.Brief{
				ProjectTitle:   "Design Doc",
				IntentSummary:  "Create the design system document.",
				TaskKind:       model.TaskKindDocument,
				TargetFilePath: targetFile,
				Goals:          []string{"Write DESIGN.md"},
				Constraints:    []string{"Ground it in the repo"},
				ReadyToStart:   true,
				OpenQuestions:  []string{},
				ManagerNotes:   "Ready to draft the document.",
			}), nil
		case "review":
			return mustMarshal(t, model.ExpertReview{
				Lens:            request.Lens,
				Summary:         "Looks good.",
				Strengths:       []string{"Clear structure"},
				Concerns:        []string{},
				Recommendations: []string{},
				BlockingRisks:   []string{},
				RequiresChanges: false,
			}), nil
		case "proposal":
			return mustMarshal(t, model.Proposal{
				Title:       "Design System Draft",
				Context:     "Define the canonical target-state design system for the terminal UI.",
				Goals:       []string{"Write DESIGN.md"},
				Constraints: []string{"Keep it accurate", "Stay in planning mode for this step; do not inspect repository files or edit files yet."},
				RecommendedPlan: []model.PlanItem{
					{Title: "Document Authority", Details: "Declare `DESIGN.md` as the canonical design-system reference for the TUI."},
					{Title: "Semantic Tokens", Details: "Define semantic tokens for text hierarchy, surfaces, borders, accent states, feedback states, spacing, and emphasis."},
				},
				Risks:               []string{},
				OpenQuestions:       []string{},
				ConsensusNotes:      []string{"Ready to write"},
				DeliverablePath:     targetFile,
				DeliverableMarkdown: "",
				Converged:           true,
				ChangeSummary:       "Finalized the markdown deliverable.",
			}), nil
		default:
			t.Fatalf("unexpected output kind %q", request.OutputKind)
			return "", nil
		}
	}

	engine := NewEngine(
		fakeProvider{id: model.ProviderCodex, run: handler},
		fakeProvider{id: model.ProviderClaude, run: handler},
		fakeProvider{id: model.ProviderGemini, run: handler},
	)

	run, err := engine.NewRun(NewRunOptions{
		CWD:             tempDir,
		OutputRoot:      filepath.Join(tempDir, "runs"),
		MaxRounds:       1,
		ManagerProvider: model.ProviderCodex,
		ExpertProviders: []model.ProviderID{model.ProviderClaude, model.ProviderGemini},
	})
	if err != nil {
		t.Fatalf("new run: %v", err)
	}

	run, err = engine.UpdateBrief(context.Background(), run, "Create DESIGN.md", nil)
	if err != nil {
		t.Fatalf("update brief: %v", err)
	}
	run, err = engine.RunDiscussion(context.Background(), run, nil)
	if err != nil {
		t.Fatalf("run discussion: %v", err)
	}

	if run.DeliverablePath != targetFile {
		t.Fatalf("expected deliverable path %q, got %q", targetFile, run.DeliverablePath)
	}
	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("read target file: %v", err)
	}
	if !strings.Contains(string(data), "# Design Doc TUI Design System") {
		t.Fatalf("expected target file to contain rendered deliverable title, got:\n%s", string(data))
	}
	if !strings.Contains(string(data), "## Document Authority") {
		t.Fatalf("expected target file to contain rendered plan sections, got:\n%s", string(data))
	}
	if strings.Contains(string(data), "Stay in planning mode") {
		t.Fatalf("expected planning-only constraint to be removed from deliverable, got:\n%s", string(data))
	}
	finalData, err := os.ReadFile(filepath.Join(run.OutputDir, "final.md"))
	if err != nil {
		t.Fatalf("read final artifact: %v", err)
	}
	if string(finalData) != string(data) {
		t.Fatalf("expected run final artifact to mirror deliverable markdown")
	}
}

func TestParseProviderOutputSupportsWrappedProviderFormats(t *testing.T) {
	type payload struct {
		OK bool `json:"ok"`
	}

	claudeRaw := `{"structured_output":{"ok":true}}`
	gotClaude, err := parseProviderOutput[payload](model.ProviderClaude, claudeRaw)
	if err != nil {
		t.Fatalf("parse claude wrapper: %v", err)
	}
	if !gotClaude.OK {
		t.Fatal("expected claude wrapper to parse structured output")
	}

	geminiRaw := "{\"response\":\"```json\\n{\\\"ok\\\":true}\\n```\"}"
	gotGemini, err := parseProviderOutput[payload](model.ProviderGemini, geminiRaw)
	if err != nil {
		t.Fatalf("parse gemini wrapper: %v", err)
	}
	if !gotGemini.OK {
		t.Fatal("expected gemini wrapper to parse response content")
	}
}

func mustMarshal(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(data)
}
