package render

import (
	"strings"
	"testing"

	"panelofexperts/internal/model"
)

func TestRenderProposalMarkdownIncludesCoreSections(t *testing.T) {
	run := model.RunState{
		ID:           "run-123",
		ProjectTitle: "Panel Project",
		Status:       model.RunStatusComplete,
		StopReason:   model.StopReasonConverged,
		Rounds:       []model.RoundState{{Round: 1}},
	}
	proposal := model.Proposal{
		Title:       "Shipping plan",
		Context:     "Important system context.",
		Goals:       []string{"Goal A"},
		Constraints: []string{"Constraint A"},
		RecommendedPlan: []model.PlanItem{
			{Title: "Step 1", Details: "Do the first thing."},
		},
		Risks:          []string{"Risk A"},
		OpenQuestions:  []string{"Question A"},
		ConsensusNotes: []string{"Consensus A"},
		ChangeSummary:  "No major changes remain.",
	}

	output := RenderProposalMarkdown(proposal, run)

	for _, expected := range []string{
		"# Shipping plan",
		"## Context",
		"## Recommended Plan",
		"## Risks",
		"## Metadata",
		"run-123",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected markdown to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestProposalHashStable(t *testing.T) {
	proposal := model.Proposal{
		Title:         "Stable",
		Context:       "Same payload.",
		ChangeSummary: "Same summary.",
	}
	if got, want := ProposalHash(proposal), ProposalHash(proposal); got != want {
		t.Fatalf("expected stable hash, got %q want %q", got, want)
	}
}
