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
		Risks:           []string{"Risk A"},
		OpenQuestions:   []string{"Question A"},
		ConsensusNotes:  []string{"Consensus A"},
		DeliverablePath: "/tmp/DESIGN.md",
		ChangeSummary:   "No major changes remain.",
	}

	output := RenderProposalMarkdown(proposal, run)

	for _, expected := range []string{
		"# Shipping plan",
		"## Context",
		"## Recommended Plan",
		"## Risks",
		"## Deliverable",
		"/tmp/DESIGN.md",
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

func TestDocumentHashTracksRenderedMarkdown(t *testing.T) {
	draft := model.DocumentDraft{
		Path:     "/tmp/DESIGN.md",
		Markdown: "# Stable document\n",
	}
	if got, want := DocumentHash(draft), DocumentHash(draft); got != want {
		t.Fatalf("expected stable document hash, got %q want %q", got, want)
	}
	if same := DocumentHash(draft); same == DocumentHash(model.DocumentDraft{Path: draft.Path, Markdown: "# Changed\n"}) {
		t.Fatalf("expected document hash to change when markdown changes")
	}
}

func TestRenderDeliverableMarkdownBuildsDocumentContent(t *testing.T) {
	run := model.RunState{
		ProjectTitle: "Panel of Experts",
		Brief: model.Brief{
			TaskKind:       model.TaskKindDocument,
			TargetFilePath: "/tmp/DESIGN.md",
			IntentSummary:  "Create the TUI app design system document.",
		},
	}
	proposal := model.Proposal{
		Context: "Planning-only proposal for `/tmp/DESIGN.md`.",
		Goals:   []string{"Define semantic tokens", "Define interaction states"},
		Constraints: []string{
			"Stay in planning mode for this step; do not inspect repository files or edit files yet.",
			"Keep the document semantic rather than framework-API-driven.",
		},
		RecommendedPlan: []model.PlanItem{
			{Title: "Document Authority", Details: "Declare the file as the canonical design-system reference."},
		},
		ConsensusNotes: []string{"Use a target-state design-system approach."},
	}

	output := RenderDeliverableMarkdown(run, proposal)

	for _, expected := range []string{
		"# Panel of Experts TUI Design System",
		"## Design Goals",
		"## Constraints",
		"## Document Authority",
		"## Consensus Notes",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected deliverable markdown to contain %q, got:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "Stay in planning mode") {
		t.Fatalf("expected planning-only text to be removed, got:\n%s", output)
	}
}

func TestRenderRepoGroundingMarkdownIncludesFactsAndFiles(t *testing.T) {
	grounding := model.RepoGrounding{
		Status:        model.RepoGroundingReady,
		WorkspaceRoot: "/tmp/panelofexperts",
		Summary:       "Go workspace using Bubble Tea.",
		Facts: []model.GroundingFact{
			{Category: "framework", Label: "Frameworks", Value: "Bubble Tea, Bubbles, Lip Gloss", EvidencePaths: []string{"go.mod"}},
			{Category: "entrypoint", Label: "CLI Entrypoints", Value: "`cmd/poe`", EvidencePaths: []string{"cmd/poe/main.go"}},
		},
		Unknowns:     []string{"No release automation file was detected."},
		ScannedFiles: []string{"go.mod", "cmd/poe/main.go"},
	}

	output := RenderRepoGroundingMarkdown(grounding)

	for _, expected := range []string{
		"# Repo Grounding",
		"/tmp/panelofexperts",
		"## Facts",
		"Bubble Tea, Bubbles, Lip Gloss",
		"## Unknowns",
		"## Scanned Files",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected repo grounding markdown to contain %q, got:\n%s", expected, output)
		}
	}
}
