package orchestrator

import (
	"testing"

	"panelofexperts/internal/model"
)

func TestSanitizeBriefForStorageRemovesPlanningArtifacts(t *testing.T) {
	brief := sanitizeBriefForStorage(model.Brief{
		Constraints: []string{
			"Use repo grounding as the baseline workspace context.",
			"Keep timelines out of scope.",
			"Do not edit files.",
		},
		ManagerNotes: "Repo grounding remains sufficient. Keep timelines out of scope.",
	})

	if got, want := brief.Constraints, []string{"Keep timelines out of scope.", "Do not edit files."}; len(got) != len(want) {
		t.Fatalf("expected sanitized constraints %v, got %v", want, got)
	}
	if brief.Constraints[0] != "Keep timelines out of scope." || brief.Constraints[1] != "Do not edit files." {
		t.Fatalf("unexpected sanitized constraints: %v", brief.Constraints)
	}
	if got, want := brief.ManagerNotes, "Keep timelines out of scope."; got != want {
		t.Fatalf("expected manager notes %q, got %q", want, got)
	}
}

func TestBriefForDocumentPromptRemovesDocumentPlanningArtifacts(t *testing.T) {
	brief := briefForDocumentPrompt(model.Brief{
		Constraints: []string{
			"Do not edit files.",
			"Keep timelines out of scope.",
		},
		ManagerNotes: "Stay in planning mode. Keep timelines out of scope.",
	})

	if got, want := brief.Constraints, []string{"Keep timelines out of scope."}; len(got) != len(want) {
		t.Fatalf("expected sanitized constraints %v, got %v", want, got)
	}
	if brief.Constraints[0] != "Keep timelines out of scope." {
		t.Fatalf("unexpected sanitized constraints: %v", brief.Constraints)
	}
	if got, want := brief.ManagerNotes, "Keep timelines out of scope."; got != want {
		t.Fatalf("expected manager notes %q, got %q", want, got)
	}
}
