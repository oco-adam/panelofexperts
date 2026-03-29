package orchestrator

import (
	"strings"
	"testing"

	"panelofexperts/internal/model"
)

func TestPlanningPromptsIncludeRepoGroundingRules(t *testing.T) {
	run := model.NewRunState(
		"run-123",
		"/workspace/panelofexperts",
		"/workspace/panelofexperts/.panel-of-experts/runs/run-123",
		5,
		model.MergeStrategyTogether,
		model.AgentConfig{ID: "manager", Name: "Manager (Codex CLI)", Role: model.RoleManager, Provider: model.ProviderCodex},
		[]model.AgentConfig{{ID: "expert-1", Name: "Expert 1", Role: model.RoleExpert, Provider: model.ProviderClaude, Lens: model.LensArchitecture}},
	)
	run.RepoGrounding = model.RepoGrounding{
		Status:        model.RepoGroundingReady,
		WorkspaceRoot: run.CWD,
		Summary:       "Go workspace using Bubble Tea.",
		Facts: []model.GroundingFact{
			{Category: "framework", Label: "Frameworks", Value: "Bubble Tea", EvidencePaths: []string{"go.mod"}},
		},
		Unknowns:     []string{},
		ScannedFiles: []string{"go.mod"},
	}
	run.Brief = model.Brief{
		ProjectTitle:  "Panel of Experts",
		IntentSummary: "Improve the manager flow.",
		TaskKind:      model.TaskKindPlan,
		Goals:         []string{"Improve planning"},
		Constraints:   []string{"Read-only"},
		OpenQuestions: []string{},
	}
	proposal := model.Proposal{
		Title:           "Grounding plan",
		Context:         "Improve repo awareness.",
		Goals:           []string{"Improve planning"},
		Constraints:     []string{"Read-only"},
		RecommendedPlan: []model.PlanItem{{Title: "Grounding", Details: "Add repo grounding."}},
		ChangeSummary:   "Initial draft.",
	}
	review := model.ExpertReview{
		Lens:            model.LensArchitecture,
		Summary:         "Looks good.",
		Strengths:       []string{},
		Concerns:        []string{},
		Recommendations: []string{},
		BlockingRisks:   []string{},
		RequiresChanges: false,
	}

	prompts := []string{
		buildBriefPrompt(run, "Improve the manager flow"),
		buildInitialProposalPrompt(run),
		buildExpertReviewPrompt(run, proposal, run.Experts[0]),
		buildMergePrompt(run, proposal, review, run.Experts[0]),
		buildCombinedMergePrompt(run, proposal, []reviewBundleItem{{Name: run.Experts[0].Name, Lens: run.Experts[0].Lens, Review: review}}),
	}

	for _, prompt := range prompts {
		for _, expected := range []string{
			"Repo grounding has already been collected",
			"Use repo grounding first.",
			"Repo grounding:",
		} {
			if !strings.Contains(prompt, expected) {
				t.Fatalf("expected prompt to contain %q, got:\n%s", expected, prompt)
			}
		}
	}
}
