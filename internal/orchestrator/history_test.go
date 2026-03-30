package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"panelofexperts/internal/model"
)

func TestResumeRunRetriesFailedDeliverableWithOverride(t *testing.T) {
	tempDir := t.TempDir()
	outputRoot := filepath.Join(tempDir, "runs")
	targetFile := filepath.Join(tempDir, "DESIGN.md")

	var sawTimeout time.Duration
	engine := NewEngine(
		fakeProvider{
			id: model.ProviderCodex,
			run: func(request model.Request) (string, error) {
				if request.OutputKind != "deliverable" {
					t.Fatalf("expected deliverable request, got %q", request.OutputKind)
				}
				sawTimeout = request.Timeout
				return mustMarshal(t, model.DocumentDraft{
					Path:     targetFile,
					Markdown: "# Retried deliverable\n",
				}), nil
			},
		},
	)

	run, err := engine.NewRun(NewRunOptions{
		CWD:                tempDir,
		OutputRoot:         outputRoot,
		DeliverableTimeout: 45 * time.Minute,
		ManagerProvider:    model.ProviderCodex,
		ExpertProviders:    []model.ProviderID{model.ProviderCodex, model.ProviderCodex},
	})
	if err != nil {
		t.Fatalf("new run: %v", err)
	}

	run.ProjectTitle = "Retry test"
	run.CurrentRound = 2
	run.Brief = model.Brief{
		ProjectTitle:   "Retry test",
		IntentSummary:  "Write the final document",
		TaskKind:       model.TaskKindDocument,
		TargetFilePath: targetFile,
		Goals:          []string{"Finish the doc"},
		ReadyToStart:   true,
	}
	run.FinalProposal = &model.Proposal{
		Title:               "Final proposal",
		Context:             "Ready for the document pass.",
		Goals:               []string{"Finish the doc"},
		RecommendedPlan:     []model.PlanItem{{Title: "Write", Details: "Write the doc."}},
		DeliverablePath:     targetFile,
		DeliverableMarkdown: "",
	}
	run.Status = model.RunStatusFailed
	run.CurrentPhase = "deliverable_draft_failed"
	run.StopReason = model.StopReasonManagerFailed
	run.FailureSummary = "codex timed out after 8m0s"
	run.PendingStatus = model.RunStatusConverged
	run.PendingStopReason = model.StopReasonConverged

	store, err := NewStore(run.OutputDir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := store.SaveState(run); err != nil {
		t.Fatalf("save state: %v", err)
	}

	resumed, err := engine.ResumeRun(context.Background(), run, ResumeRunOptions{
		DeliverableTimeout: 2 * time.Hour,
	}, nil)
	if err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if sawTimeout != 2*time.Hour {
		t.Fatalf("expected deliverable timeout override of 2h, got %s", sawTimeout)
	}
	if resumed.DeliverableTimeout != 2*time.Hour {
		t.Fatalf("expected run to persist 2h deliverable timeout, got %s", resumed.DeliverableTimeout)
	}
	if resumed.Status != model.RunStatusConverged {
		t.Fatalf("expected resumed run to restore converged status, got %s", resumed.Status)
	}
	if resumed.StopReason != model.StopReasonConverged {
		t.Fatalf("expected resumed run to restore converged stop reason, got %s", resumed.StopReason)
	}
	if resumed.CurrentPhase != "finalized" {
		t.Fatalf("expected finalized phase, got %s", resumed.CurrentPhase)
	}
	if resumed.PendingStatus != "" || resumed.PendingStopReason != "" {
		t.Fatalf("expected pending outcome fields to be cleared, got status=%q stop=%q", resumed.PendingStatus, resumed.PendingStopReason)
	}
	if _, err := os.Stat(targetFile); err != nil {
		t.Fatalf("expected deliverable file to be written: %v", err)
	}
}

func TestRerunFromRunClonesBriefIntoFreshRunWithOverrides(t *testing.T) {
	tempDir := t.TempDir()
	outputRoot := filepath.Join(tempDir, "runs")

	handler := func(request model.Request) (string, error) {
		switch request.OutputKind {
		case "review":
			return mustMarshal(t, model.ExpertReview{
				Lens:            request.Lens,
				Summary:         "Looks good.",
				Strengths:       []string{"Ready to finalize"},
				Concerns:        []string{},
				Recommendations: []string{},
				BlockingRisks:   []string{},
				RequiresChanges: false,
			}), nil
		case "proposal":
			return mustMarshal(t, model.Proposal{
				Title:           "Fresh rerun proposal",
				Context:         "Built from the saved brief.",
				Goals:           []string{"Ship the rerun"},
				Constraints:     []string{},
				RecommendedPlan: []model.PlanItem{{Title: "Ship", Details: "Ship it."}},
				Risks:           []string{},
				OpenQuestions:   []string{},
				ConsensusNotes:  []string{"No changes required"},
				Converged:       true,
				ChangeSummary:   "Immediate convergence.",
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

	source, err := engine.NewRun(NewRunOptions{
		CWD:             tempDir,
		OutputRoot:      outputRoot,
		MaxRounds:       5,
		MergeStrategy:   model.MergeStrategyTogether,
		ManagerProvider: model.ProviderCodex,
		ExpertProviders: []model.ProviderID{model.ProviderClaude, model.ProviderGemini},
	})
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	source.Brief = model.Brief{
		ProjectTitle:  "Saved brief",
		IntentSummary: "Reuse this brief",
		TaskKind:      model.TaskKindPlan,
		Goals:         []string{"Ship the rerun"},
		Constraints:   []string{"Keep it concise"},
		ReadyToStart:  true,
		OpenQuestions: []string{},
		ManagerNotes:  "Good to go",
	}
	source.ManagerTurns = []model.ManagerTurn{{
		UserMessage:  "Reuse the previous brief",
		BriefSummary: "Reuse this brief",
	}}

	rerun, err := engine.RerunFromRun(context.Background(), source, RerunOptions{
		MaxRounds:          3,
		MergeStrategy:      model.MergeStrategySequential,
		ManagerProvider:    model.ProviderClaude,
		ExpertProviders:    []model.ProviderID{model.ProviderGemini, model.ProviderCodex},
		DeliverableTimeout: 90 * time.Minute,
	}, nil)
	if err != nil {
		t.Fatalf("rerun from run: %v", err)
	}
	if rerun.ID == source.ID {
		t.Fatal("expected rerun to create a fresh run id")
	}
	if rerun.OutputDir == source.OutputDir {
		t.Fatal("expected rerun to use a new output directory")
	}
	if rerun.Manager.Provider != model.ProviderClaude {
		t.Fatalf("expected overridden manager provider, got %s", rerun.Manager.Provider)
	}
	if rerun.MergeStrategy != model.MergeStrategySequential {
		t.Fatalf("expected sequential merge strategy, got %s", rerun.MergeStrategy)
	}
	if rerun.MaxRounds != 3 {
		t.Fatalf("expected overridden max rounds, got %d", rerun.MaxRounds)
	}
	if rerun.DeliverableTimeout != 90*time.Minute {
		t.Fatalf("expected overridden deliverable timeout, got %s", rerun.DeliverableTimeout)
	}
	if len(rerun.Experts) != 2 || rerun.Experts[0].Provider != model.ProviderGemini || rerun.Experts[1].Provider != model.ProviderCodex {
		t.Fatalf("expected overridden expert providers, got %+v", rerun.Experts)
	}
	if rerun.Status != model.RunStatusConverged {
		t.Fatalf("expected rerun to complete and converge, got %s", rerun.Status)
	}
	if rerun.FinalProposal == nil || rerun.FinalProposal.Title != "Fresh rerun proposal" {
		t.Fatalf("expected rerun final proposal to come from the rerun execution, got %+v", rerun.FinalProposal)
	}
}
