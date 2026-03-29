package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
					Context:             "Context after combined expert review.",
					Goals:               []string{"Plan the app"},
					Constraints:         []string{"Read-only"},
					RecommendedPlan:     []model.PlanItem{{Title: "Refine", Details: "Add live status board."}},
					Risks:               []string{"Need deterministic convergence"},
					OpenQuestions:       []string{},
					ConsensusNotes:      []string{"Combined expert feedback merged"},
					DeliverablePath:     "",
					DeliverableMarkdown: "",
					Converged:           true,
					ChangeSummary:       "Merged expert bundle; proposal converged.",
				}), nil
			default:
				t.Fatalf("unexpected proposal version %d", request.Version)
				return "", nil
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
	if run.MergeStrategy != model.MergeStrategyTogether {
		t.Fatalf("expected default merge strategy to be together, got %s", run.MergeStrategy)
	}
	if run.AgentStatuses["expert-1"].State != model.AgentStateDone {
		t.Fatalf("expected expert-1 to be done after merge, got %s", run.AgentStatuses["expert-1"].State)
	}
	if run.AgentStatuses["expert-2"].State != model.AgentStateDone {
		t.Fatalf("expected expert-2 to be done after merge, got %s", run.AgentStatuses["expert-2"].State)
	}

	for _, rel := range []string{
		"repo-grounding.json",
		"repo-grounding.md",
		"brief.json",
		"brief.md",
		"proposal-v001.json",
		"proposal-v002.json",
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
	if !strings.Contains(string(finalData), "# Merged proposal") {
		t.Fatalf("expected final markdown to contain final proposal title, got:\n%s", string(finalData))
	}
}

func TestEngineWritesDocumentDeliverableToTargetFile(t *testing.T) {
	tempDir := t.TempDir()
	targetFile := filepath.Join(tempDir, "DESIGN.md")
	finalDocument := strings.TrimSpace(`
# Design Doc TUI Design System

This document defines the terminal UI design system for the app.

## Document Authority

` + "`DESIGN.md`" + ` is the canonical reference for shared terminal UI patterns and screen behavior.

## Semantic Tokens

Use semantic roles for typography, borders, spacing, emphasis, and feedback states so implementation details can evolve without changing the contract.
`)
	sawDeliverable := false

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
		case "deliverable":
			sawDeliverable = true
			if !strings.Contains(strings.ToLower(request.Prompt), "this is no longer a planning or proposal step") {
				t.Fatalf("expected deliverable prompt to explicitly leave planning mode, got:\n%s", request.Prompt)
			}
			return mustMarshal(t, model.DocumentDraft{
				Path:     targetFile,
				Markdown: finalDocument,
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
	if !sawDeliverable {
		t.Fatalf("expected manager to run a final deliverable drafting pass")
	}

	if run.DeliverablePath != targetFile {
		t.Fatalf("expected deliverable path %q, got %q", targetFile, run.DeliverablePath)
	}
	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("read target file: %v", err)
	}
	if strings.TrimSpace(string(data)) != finalDocument {
		t.Fatalf("expected target file to contain the drafted final document, got:\n%s", string(data))
	}
	finalData, err := os.ReadFile(filepath.Join(run.OutputDir, "final.md"))
	if err != nil {
		t.Fatalf("read final artifact: %v", err)
	}
	if string(finalData) != string(data) {
		t.Fatalf("expected run final artifact to mirror deliverable markdown")
	}
}

func TestDocumentRunPersistsRunningStatusWhileDraftingDeliverable(t *testing.T) {
	tempDir := t.TempDir()
	targetFile := filepath.Join(tempDir, "migrate-to-rust-impl-plan.md")
	sawWritingSnapshot := false

	handler := func(request model.Request) (string, error) {
		switch request.OutputKind {
		case "brief":
			return mustMarshal(t, model.Brief{
				ProjectTitle:   "Theda",
				IntentSummary:  "Rewrite the migration plan.",
				TaskKind:       model.TaskKindDocument,
				TargetFilePath: targetFile,
				Goals:          []string{"Rewrite the document"},
				Constraints:    []string{"Stay grounded in the repo"},
				ReadyToStart:   true,
				OpenQuestions:  []string{},
				ManagerNotes:   "Ready to write the final document.",
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
				Title:               "Migration plan rewrite",
				Context:             "Rewrite the migration plan as the final document.",
				Goals:               []string{"Rewrite the document"},
				Constraints:         []string{"Ground it in the repo"},
				RecommendedPlan:     []model.PlanItem{{Title: "Rewrite", Details: "Draft the final markdown."}},
				Risks:               []string{},
				OpenQuestions:       []string{},
				ConsensusNotes:      []string{"Ready to write"},
				DeliverablePath:     targetFile,
				DeliverableMarkdown: "",
				Converged:           true,
				ChangeSummary:       "Ready for final drafting.",
			}), nil
		case "deliverable":
			return mustMarshal(t, model.DocumentDraft{
				Path:     targetFile,
				Markdown: "# Final Migration Plan\n",
			}), nil
		default:
			t.Fatalf("unexpected output kind %q", request.OutputKind)
			return "", nil
		}
	}

	engine := NewEngine(
		fakeProvider{id: model.ProviderCodex, run: handler},
		fakeProvider{id: model.ProviderClaude, run: handler},
	)

	run, err := engine.NewRun(NewRunOptions{
		CWD:             tempDir,
		OutputRoot:      filepath.Join(tempDir, "runs"),
		MaxRounds:       1,
		ManagerProvider: model.ProviderCodex,
		ExpertProviders: []model.ProviderID{model.ProviderClaude},
	})
	if err != nil {
		t.Fatalf("new run: %v", err)
	}

	run, err = engine.UpdateBrief(context.Background(), run, "Rewrite the migration plan", nil)
	if err != nil {
		t.Fatalf("update brief: %v", err)
	}

	run, err = engine.RunDiscussion(context.Background(), run, func(snapshot model.RunState) {
		if sawWritingSnapshot || snapshot.CurrentPhase != "writing_deliverable" {
			return
		}
		sawWritingSnapshot = true

		data, readErr := os.ReadFile(filepath.Join(snapshot.OutputDir, "state.json"))
		if readErr != nil {
			t.Fatalf("read persisted state: %v", readErr)
		}
		var persisted model.RunState
		if unmarshalErr := json.Unmarshal(data, &persisted); unmarshalErr != nil {
			t.Fatalf("unmarshal persisted state: %v", unmarshalErr)
		}
		if persisted.Status != model.RunStatusRunning {
			t.Fatalf("expected persisted status to remain running while drafting deliverable, got %s", persisted.Status)
		}
		if persisted.CurrentPhase != "writing_deliverable" {
			t.Fatalf("expected persisted phase writing_deliverable, got %s", persisted.CurrentPhase)
		}
		if persisted.AgentStatuses[persisted.Manager.ID].State != model.AgentStateRunning {
			t.Fatalf("expected manager to remain running while drafting deliverable, got %s", persisted.AgentStatuses[persisted.Manager.ID].State)
		}
	})
	if err != nil {
		t.Fatalf("run discussion: %v", err)
	}
	if !sawWritingSnapshot {
		t.Fatal("expected to observe a persisted writing_deliverable snapshot")
	}
	if run.Status != model.RunStatusConverged {
		t.Fatalf("expected completed document run to converge, got %s", run.Status)
	}
}

func TestEngineNewRunDefaultsMaxRoundsToFive(t *testing.T) {
	tempDir := t.TempDir()
	engine := NewEngine(
		fakeProvider{id: model.ProviderCodex, run: func(request model.Request) (string, error) { return "{}", nil }},
	)

	run, err := engine.NewRun(NewRunOptions{
		CWD:             tempDir,
		OutputRoot:      filepath.Join(tempDir, "runs"),
		ManagerProvider: model.ProviderCodex,
		ExpertProviders: []model.ProviderID{model.ProviderCodex, model.ProviderCodex},
	})
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if run.MaxRounds != 5 {
		t.Fatalf("expected default max rounds to be 5, got %d", run.MaxRounds)
	}
	if run.MergeStrategy != model.MergeStrategyTogether {
		t.Fatalf("expected default merge strategy to be together, got %s", run.MergeStrategy)
	}
}

func TestRunDiscussionPublishesCompletedExpertStateBeforeAllReviewsFinish(t *testing.T) {
	tempDir := t.TempDir()
	releaseSlowReview := make(chan struct{})
	sawIntermediate := false

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
				OpenQuestions:  []string{},
				ManagerNotes:   "Ready for expert review.",
			}), nil
		case "review":
			if request.AgentID == "expert-2" {
				<-releaseSlowReview
			}
			return mustMarshal(t, model.ExpertReview{
				Lens:            request.Lens,
				Summary:         fmt.Sprintf("%s review complete", request.AgentID),
				Strengths:       []string{"Clear structure"},
				Concerns:        []string{},
				Recommendations: []string{},
				BlockingRisks:   []string{},
				RequiresChanges: false,
			}), nil
		case "proposal":
			return mustMarshal(t, model.Proposal{
				Title:               "Stable proposal",
				Context:             "Planning context.",
				Goals:               []string{"Plan the app"},
				Constraints:         []string{"Read-only"},
				RecommendedPlan:     []model.PlanItem{{Title: "Ship", Details: "Implement the approved plan."}},
				Risks:               []string{},
				OpenQuestions:       []string{},
				ConsensusNotes:      []string{"Panel converged"},
				DeliverablePath:     "",
				DeliverableMarkdown: "",
				Converged:           false,
				ChangeSummary:       fmt.Sprintf("manager version %d", request.Version),
			}), nil
		default:
			t.Fatalf("unexpected output kind %q", request.OutputKind)
			return "", nil
		}
	}

	engine := NewEngine(
		fakeProvider{id: model.ProviderCodex, run: handler},
		fakeProvider{id: model.ProviderClaude, run: handler},
	)

	run, err := engine.NewRun(NewRunOptions{
		CWD:             tempDir,
		OutputRoot:      filepath.Join(tempDir, "runs"),
		MaxRounds:       1,
		ManagerProvider: model.ProviderCodex,
		ExpertProviders: []model.ProviderID{model.ProviderCodex, model.ProviderClaude},
	})
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	run, err = engine.UpdateBrief(context.Background(), run, "Build the app", nil)
	if err != nil {
		t.Fatalf("update brief: %v", err)
	}

	_, err = engine.RunDiscussion(context.Background(), run, func(snapshot model.RunState) {
		fast := snapshot.AgentStatuses["expert-1"]
		slow := snapshot.AgentStatuses["expert-2"]
		if !sawIntermediate && fast.State == model.AgentStateDone && slow.State == model.AgentStateRunning {
			sawIntermediate = true
			close(releaseSlowReview)
		}
	})
	if err != nil {
		t.Fatalf("run discussion: %v", err)
	}
	if !sawIntermediate {
		close(releaseSlowReview)
		t.Fatal("expected an intermediate snapshot with one expert done while another review was still running")
	}
}

func TestRunExpertReviewRetriesWithoutWorkspaceAccess(t *testing.T) {
	attempts := []model.Request{}
	provider := fakeProvider{
		id: model.ProviderClaude,
		run: func(request model.Request) (string, error) {
			attempts = append(attempts, request)
			if len(attempts) < 3 {
				return "", errors.New("claude timed out after 5m0s")
			}
			if request.Metadata["workspace_access"] != "none" {
				t.Fatalf("expected final retry to disable workspace access, got metadata=%v", request.Metadata)
			}
			if !strings.Contains(request.Prompt, "Retry mode: do not inspect the repository or use tools.") {
				t.Fatalf("expected retry prompt to include fallback instruction, got:\n%s", request.Prompt)
			}
			return mustMarshal(t, model.ExpertReview{
				Lens:            model.LensArchitecture,
				Summary:         "Recovered on retry.",
				Strengths:       []string{},
				Concerns:        []string{},
				Recommendations: []string{},
				BlockingRisks:   []string{},
				RequiresChanges: false,
			}), nil
		},
	}

	engine := NewEngine(provider)
	review, err := engine.runExpertReview(context.Background(), provider, model.Request{
		RunID:      "run-1",
		Round:      1,
		AgentID:    "expert-1",
		Role:       model.RoleExpert,
		Prompt:     "Review the proposal",
		JSONSchema: reviewSchema,
		OutputKind: "review",
		Timeout:    time.Minute,
	}, make(chan model.ProgressEvent, 8))
	if err != nil {
		t.Fatalf("run expert review: %v", err)
	}
	if len(attempts) != 3 {
		t.Fatalf("expected three attempts, got %d", len(attempts))
	}
	if !strings.Contains(attempts[1].Prompt, "Retry attempt 2 of 3.") {
		t.Fatalf("expected second attempt to include strict retry instructions, got:\n%s", attempts[1].Prompt)
	}
	if review.Summary != "Recovered on retry." {
		t.Fatalf("expected retry review to be returned, got %+v", review)
	}
}

func TestUpdateBriefRetriesManagerRequests(t *testing.T) {
	tempDir := t.TempDir()
	attempts := 0

	engine := NewEngine(fakeProvider{
		id: model.ProviderCodex,
		run: func(request model.Request) (string, error) {
			attempts++
			if request.OutputKind != "brief" {
				t.Fatalf("unexpected output kind %q", request.OutputKind)
			}
			if attempts < 3 {
				return "not-json", nil
			}
			return mustMarshal(t, model.Brief{
				ProjectTitle:   "Panel Test Project",
				IntentSummary:  "Build the planning workflow.",
				TaskKind:       model.TaskKindPlan,
				TargetFilePath: "",
				Goals:          []string{"Plan the app"},
				Constraints:    []string{"Read-only"},
				ReadyToStart:   true,
				OpenQuestions:  []string{},
				ManagerNotes:   "Ready for expert review.",
			}), nil
		},
	})

	run, err := engine.NewRun(NewRunOptions{
		CWD:             tempDir,
		OutputRoot:      filepath.Join(tempDir, "runs"),
		MaxRounds:       1,
		ManagerProvider: model.ProviderCodex,
		ExpertProviders: []model.ProviderID{model.ProviderCodex},
	})
	if err != nil {
		t.Fatalf("new run: %v", err)
	}

	run, err = engine.UpdateBrief(context.Background(), run, "Build the app", nil)
	if err != nil {
		t.Fatalf("update brief: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected three manager brief attempts, got %d", attempts)
	}
	if run.Brief.ProjectTitle != "Panel Test Project" {
		t.Fatalf("expected brief to succeed after retries, got %+v", run.Brief)
	}
}

func TestUpdateBriefAllowsManagerRepositoryInspection(t *testing.T) {
	tempDir := t.TempDir()

	engine := NewEngine(fakeProvider{
		id: model.ProviderCodex,
		run: func(request model.Request) (string, error) {
			if request.Metadata["workspace_access"] == "none" {
				t.Fatalf("expected brief request to keep workspace access enabled")
			}
			for _, expected := range []string{
				"Repo grounding has already been collected",
				"Inspect repository files only when the grounding leaves a real gap",
				"Repo grounding:",
			} {
				if !strings.Contains(request.Prompt, expected) {
					t.Fatalf("expected brief prompt to include %q, got:\n%s", expected, request.Prompt)
				}
			}
			return mustMarshal(t, model.Brief{
				ProjectTitle:   "Panel Test Project",
				IntentSummary:  "Build the planning workflow.",
				TaskKind:       model.TaskKindPlan,
				TargetFilePath: "",
				Goals:          []string{"Plan the app"},
				Constraints:    []string{"Read-only"},
				ReadyToStart:   true,
				OpenQuestions:  []string{},
				ManagerNotes:   "Ready for expert review.",
			}), nil
		},
	})

	run, err := engine.NewRun(NewRunOptions{
		CWD:             tempDir,
		OutputRoot:      filepath.Join(tempDir, "runs"),
		MaxRounds:       1,
		ManagerProvider: model.ProviderCodex,
		ExpertProviders: []model.ProviderID{model.ProviderCodex},
	})
	if err != nil {
		t.Fatalf("new run: %v", err)
	}

	if _, err := engine.UpdateBrief(context.Background(), run, "Build the app", nil); err != nil {
		t.Fatalf("update brief: %v", err)
	}
}

func TestUpdateBriefWritesAndReusesRepoGrounding(t *testing.T) {
	tempDir := t.TempDir()
	writeGroundingFixture(t, tempDir)

	prompts := []string{}
	engine := NewEngine(fakeProvider{
		id: model.ProviderCodex,
		run: func(request model.Request) (string, error) {
			prompts = append(prompts, request.Prompt)
			if !strings.Contains(request.Prompt, "Bubble Tea") {
				t.Fatalf("expected prompt to include Bubble Tea grounding, got:\n%s", request.Prompt)
			}
			return mustMarshal(t, model.Brief{
				ProjectTitle:   "Panel Test Project",
				IntentSummary:  "Build the planning workflow.",
				TaskKind:       model.TaskKindPlan,
				TargetFilePath: "",
				Goals:          []string{"Plan the app"},
				Constraints:    []string{"Read-only"},
				ReadyToStart:   true,
				OpenQuestions:  []string{},
				ManagerNotes:   "Ready for expert review.",
			}), nil
		},
	})

	run, err := engine.NewRun(NewRunOptions{
		CWD:             tempDir,
		OutputRoot:      filepath.Join(tempDir, "runs"),
		MaxRounds:       1,
		ManagerProvider: model.ProviderCodex,
		ExpertProviders: []model.ProviderID{model.ProviderCodex},
	})
	if err != nil {
		t.Fatalf("new run: %v", err)
	}

	run, err = engine.UpdateBrief(context.Background(), run, "Build the app", nil)
	if err != nil {
		t.Fatalf("first update brief: %v", err)
	}
	if run.RepoGrounding.Status != model.RepoGroundingReady {
		t.Fatalf("expected ready repo grounding, got %+v", run.RepoGrounding)
	}
	if len(run.RepoGrounding.Facts) == 0 {
		t.Fatalf("expected repo grounding facts to be collected, got %+v", run.RepoGrounding)
	}
	for _, rel := range []string{"repo-grounding.json", "repo-grounding.md"} {
		if _, err := os.Stat(filepath.Join(run.OutputDir, rel)); err != nil {
			t.Fatalf("expected artifact %s: %v", rel, err)
		}
	}

	firstSummary := run.RepoGrounding.Summary
	run, err = engine.UpdateBrief(context.Background(), run, "Clarify the plan", nil)
	if err != nil {
		t.Fatalf("second update brief: %v", err)
	}
	if run.RepoGrounding.Summary != firstSummary {
		t.Fatalf("expected grounding summary to be reused, got %q want %q", run.RepoGrounding.Summary, firstSummary)
	}
	if len(prompts) != 2 {
		t.Fatalf("expected two brief prompts, got %d", len(prompts))
	}
}

func TestUpdateBriefRejectsRepoAnswerableManagerQuestions(t *testing.T) {
	tempDir := t.TempDir()
	writeGroundingFixture(t, tempDir)

	attempts := 0
	engine := NewEngine(fakeProvider{
		id: model.ProviderCodex,
		run: func(request model.Request) (string, error) {
			attempts++
			if attempts == 2 && !strings.Contains(request.Prompt, "Questions to remove or replace:") {
				t.Fatalf("expected retry prompt to explain grounded question rejection, got:\n%s", request.Prompt)
			}
			if attempts < 3 {
				return mustMarshal(t, model.Brief{
					ProjectTitle:   "Panel Test Project",
					IntentSummary:  "Build the planning workflow.",
					TaskKind:       model.TaskKindPlan,
					TargetFilePath: "",
					Goals:          []string{"Plan the app"},
					Constraints:    []string{"Read-only"},
					ReadyToStart:   false,
					OpenQuestions:  []string{"What TUI framework does the app currently use?"},
					ManagerNotes:   "Need a repo fact from the user.",
				}), nil
			}
			return mustMarshal(t, model.Brief{
				ProjectTitle:   "Panel Test Project",
				IntentSummary:  "Build the planning workflow.",
				TaskKind:       model.TaskKindPlan,
				TargetFilePath: "",
				Goals:          []string{"Plan the app"},
				Constraints:    []string{"Read-only"},
				ReadyToStart:   true,
				OpenQuestions:  []string{},
				ManagerNotes:   "Ready for expert review.",
			}), nil
		},
	})

	run, err := engine.NewRun(NewRunOptions{
		CWD:             tempDir,
		OutputRoot:      filepath.Join(tempDir, "runs"),
		MaxRounds:       1,
		ManagerProvider: model.ProviderCodex,
		ExpertProviders: []model.ProviderID{model.ProviderCodex},
	})
	if err != nil {
		t.Fatalf("new run: %v", err)
	}

	run, err = engine.UpdateBrief(context.Background(), run, "Build the app", nil)
	if err != nil {
		t.Fatalf("update brief: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected grounded-question retries, got %d attempts", attempts)
	}
	if len(run.Brief.OpenQuestions) != 0 {
		t.Fatalf("expected grounded question to be removed before success, got %+v", run.Brief.OpenQuestions)
	}
}

func TestRunDiscussionSequentialMergeStrategyPreservesPerReviewManagerPasses(t *testing.T) {
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
				OpenQuestions:  []string{},
				ManagerNotes:   "Ready for expert review.",
			}), nil
		case "review":
			return mustMarshal(t, model.ExpertReview{
				Lens:            request.Lens,
				Summary:         "Looks good.",
				Strengths:       []string{"Clear structure"},
				Concerns:        []string{},
				Recommendations: []string{},
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
					Risks:               []string{},
					OpenQuestions:       []string{},
					ConsensusNotes:      []string{},
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
					Risks:               []string{},
					OpenQuestions:       []string{},
					ConsensusNotes:      []string{"Architecture feedback merged"},
					DeliverablePath:     "",
					DeliverableMarkdown: "",
					Converged:           false,
					ChangeSummary:       "Merged architecture feedback.",
				}), nil
			case 3:
				return mustMarshal(t, model.Proposal{
					Title:               "Final proposal",
					Context:             "Context after all reviews.",
					Goals:               []string{"Plan the app"},
					Constraints:         []string{"Read-only"},
					RecommendedPlan:     []model.PlanItem{{Title: "Ship", Details: "Implement the approved plan."}},
					Risks:               []string{},
					OpenQuestions:       []string{},
					ConsensusNotes:      []string{"Panel converged"},
					DeliverablePath:     "",
					DeliverableMarkdown: "",
					Converged:           true,
					ChangeSummary:       "Execution feedback merged; proposal converged.",
				}), nil
			default:
				t.Fatalf("unexpected proposal version %d", request.Version)
				return "", nil
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
		MaxRounds:       1,
		MergeStrategy:   model.MergeStrategySequential,
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
	run, err = engine.RunDiscussion(context.Background(), run, nil)
	if err != nil {
		t.Fatalf("run discussion: %v", err)
	}
	if run.FinalProposal == nil {
		t.Fatal("expected final proposal to be present")
	}
	for _, rel := range []string{
		"proposal-v001.json",
		"proposal-v002.json",
		"proposal-v003.json",
	} {
		if _, err := os.Stat(filepath.Join(run.OutputDir, rel)); err != nil {
			t.Fatalf("expected artifact %s: %v", rel, err)
		}
	}
}

func TestRunDiscussionSurfacesManagerMergeFailureReason(t *testing.T) {
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
				OpenQuestions:  []string{},
				ManagerNotes:   "Ready for expert review.",
			}), nil
		case "review":
			return mustMarshal(t, model.ExpertReview{
				Lens:            request.Lens,
				Summary:         "Needs one more manager pass.",
				Strengths:       []string{"Clear structure"},
				Concerns:        []string{"Needs one more change"},
				Recommendations: []string{"Refine the plan"},
				BlockingRisks:   []string{},
				RequiresChanges: true,
			}), nil
		case "proposal":
			if request.Version == 1 {
				return mustMarshal(t, model.Proposal{
					Title:               "Initial proposal",
					Context:             "Initial context.",
					Goals:               []string{"Plan the app"},
					Constraints:         []string{"Read-only"},
					RecommendedPlan:     []model.PlanItem{{Title: "Draft", Details: "Create the first proposal."}},
					Risks:               []string{},
					OpenQuestions:       []string{},
					ConsensusNotes:      []string{},
					DeliverablePath:     "",
					DeliverableMarkdown: "",
					Converged:           false,
					ChangeSummary:       "Initial manager draft.",
				}), nil
			}
			return "", errors.New("codex timed out after 5m0s")
		default:
			t.Fatalf("unexpected output kind %q", request.OutputKind)
			return "", nil
		}
	}

	engine := NewEngine(
		fakeProvider{id: model.ProviderCodex, run: handler},
	)

	run, err := engine.NewRun(NewRunOptions{
		CWD:             tempDir,
		OutputRoot:      filepath.Join(tempDir, "runs"),
		MaxRounds:       1,
		ManagerProvider: model.ProviderCodex,
		ExpertProviders: []model.ProviderID{model.ProviderCodex},
	})
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	run, err = engine.UpdateBrief(context.Background(), run, "Build the app", nil)
	if err != nil {
		t.Fatalf("update brief: %v", err)
	}

	run, err = engine.RunDiscussion(context.Background(), run, nil)
	if err == nil {
		t.Fatal("expected run discussion to fail on manager merge")
	}
	if run.Status != model.RunStatusFailed {
		t.Fatalf("expected failed run status, got %s", run.Status)
	}
	if run.CurrentPhase != "manager_merge_failed" {
		t.Fatalf("expected manager_merge_failed phase, got %s", run.CurrentPhase)
	}
	if run.FailureSummary != "codex timed out after 5m0s" {
		t.Fatalf("expected failure summary to persist, got %q", run.FailureSummary)
	}
	if run.WaitingSummary != "codex timed out after 5m0s" {
		t.Fatalf("expected waiting summary to surface failure reason, got %q", run.WaitingSummary)
	}
	managerStatus := run.AgentStatuses[run.Manager.ID]
	if managerStatus.State != model.AgentStateError {
		t.Fatalf("expected manager state error, got %s", managerStatus.State)
	}
	if managerStatus.LastStep != "merge_failed" {
		t.Fatalf("expected manager failure step merge_failed, got %q", managerStatus.LastStep)
	}
	if managerStatus.Summary != "codex timed out after 5m0s" {
		t.Fatalf("expected manager failure summary, got %q", managerStatus.Summary)
	}
	foundTimelineFailure := false
	for _, entry := range run.Timeline {
		if strings.Contains(entry.Text, "Manager (Codex CLI) failed: codex timed out after 5m0s") {
			foundTimelineFailure = true
			break
		}
	}
	if !foundTimelineFailure {
		t.Fatalf("expected timeline to include merge failure, got %+v", run.Timeline)
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

	noisyGeminiRaw := strings.TrimSpace(`
Loaded cached credentials.
Loading extension: firebase
Capabilities: {
  "logging": {}
}
{
  "session_id": "abc",
  "response": "{\"ok\":true}",
  "stats": {
    "models": {}
  }
}`)
	gotNoisyGemini, err := parseProviderOutput[payload](model.ProviderGemini, noisyGeminiRaw)
	if err != nil {
		t.Fatalf("parse noisy gemini wrapper: %v", err)
	}
	if !gotNoisyGemini.OK {
		t.Fatal("expected noisy gemini wrapper to parse response content")
	}
}

func TestReviewTimeoutForExpertsUsesConfiguredDurations(t *testing.T) {
	if defaultExpertReviewTimeout != 10*time.Minute {
		t.Fatalf("expected default expert timeout to be 10m, got %s", defaultExpertReviewTimeout)
	}
	if got := reviewTimeoutFor(model.ProviderClaude); got != claudeExpertReviewTimeout {
		t.Fatalf("expected claude timeout %s, got %s", claudeExpertReviewTimeout, got)
	}
	if got := reviewTimeoutFor(model.ProviderCodex); got != defaultExpertReviewTimeout {
		t.Fatalf("expected default timeout %s for codex, got %s", defaultExpertReviewTimeout, got)
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

func writeGroundingFixture(t *testing.T, root string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(root, "go.mod"), `module panelofexperts

go 1.25.1

require (
	charm.land/bubbles/v2 v2.1.0
	charm.land/bubbletea/v2 v2.0.2
	charm.land/lipgloss/v2 v2.0.2
)
`)
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Panel of Experts\n")
	mustWriteFile(t, filepath.Join(root, "cmd", "poe", "main.go"), "package main\nfunc main() {}\n")
	mustWriteFile(t, filepath.Join(root, "internal", "ui", "model_test.go"), "package ui\n")
	mustWriteFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), "name: ci\n")
	mustWriteFile(t, filepath.Join(root, ".goreleaser.yaml"), "project_name: poe\n")
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
