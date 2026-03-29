package orchestrator

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"panelofexperts/internal/model"
)

func TestCollectRepoGroundingDetectsHighSignalWorkspaceFacts(t *testing.T) {
	tempDir := t.TempDir()
	writeGroundingFixture(t, tempDir)

	grounding, err := collectRepoGrounding(tempDir)
	if err != nil {
		t.Fatalf("collect repo grounding: %v", err)
	}
	if grounding.Status != model.RepoGroundingReady {
		t.Fatalf("expected ready grounding, got %+v", grounding)
	}
	if !strings.Contains(grounding.Summary, "Go workspace") {
		t.Fatalf("expected summary to mention Go workspace, got %q", grounding.Summary)
	}

	assertGroundingFact(t, grounding, "framework", "Bubble Tea")
	assertGroundingFact(t, grounding, "entrypoint", "cmd/poe")
	assertGroundingFact(t, grounding, "release_tooling", "GitHub Actions")
	assertGroundingFact(t, grounding, "release_tooling", "GoReleaser")
	if len(grounding.ScannedFiles) == 0 {
		t.Fatalf("expected scanned files to be recorded, got %+v", grounding)
	}
}

func TestCollectRepoGroundingDetectsMixedWorkspaceFacts(t *testing.T) {
	tempDir := t.TempDir()
	mustWriteFile(t, filepath.Join(tempDir, "dune-project"), "(lang dune 3.11)\n")
	mustWriteFile(t, filepath.Join(tempDir, "rust", "Cargo.toml"), "[workspace]\nmembers = []\n")
	mustWriteFile(t, filepath.Join(tempDir, "website", "package.json"), "{\n  \"name\": \"website\"\n}\n")
	mustWriteFile(t, filepath.Join(tempDir, "website", "tsconfig.json"), "{\n  \"compilerOptions\": {}\n}\n")
	mustWriteFile(t, filepath.Join(tempDir, "README.md"), "# Theda\n")
	mustWriteFile(t, filepath.Join(tempDir, "NORTH_STAR.md"), "# North Star\n")
	mustWriteFile(t, filepath.Join(tempDir, "spec", "public-contract", "README.md"), "# Public Contract\n")

	grounding, err := collectRepoGrounding(tempDir)
	if err != nil {
		t.Fatalf("collect repo grounding: %v", err)
	}

	for _, expected := range []string{"OCaml", "Rust", "JavaScript/TypeScript"} {
		assertGroundingFact(t, grounding, "language_runtime", expected)
	}
	assertGroundingFact(t, grounding, "manifest", "dune-project")
	assertGroundingFact(t, grounding, "manifest", "rust/Cargo.toml")
	assertGroundingFact(t, grounding, "manifest", "website/package.json")
	assertGroundingFact(t, grounding, "project_layout", "Rust workspace (`rust/Cargo.toml`)")
	assertGroundingFact(t, grounding, "project_layout", "JavaScript/TypeScript app (`website/package.json`)")
	assertGroundingFact(t, grounding, "docs", "NORTH_STAR.md")
	assertGroundingFact(t, grounding, "docs", "spec/public-contract/README.md")
	if strings.Contains(strings.Join(grounding.Unknowns, "\n"), "No CLI entrypoint was detected under cmd/.") {
		t.Fatalf("did not expect Go-specific entrypoint warning for mixed non-Go fixture, got %+v", grounding.Unknowns)
	}
	if !strings.Contains(grounding.Summary, "projects:") {
		t.Fatalf("expected summary to mention workspace projects, got %q", grounding.Summary)
	}
}

func TestValidateGroundedQuestionsRejectsRepoAnswerableQuestions(t *testing.T) {
	grounding := model.RepoGrounding{
		Status: model.RepoGroundingReady,
		Facts: []model.GroundingFact{
			{Category: "framework", Label: "Frameworks", Value: "Bubble Tea", EvidencePaths: []string{"go.mod"}},
		},
	}
	brief := model.Brief{
		OpenQuestions: []string{"What TUI framework does the app currently use?"},
	}

	err := validateGroundedQuestions(brief, grounding)
	if err == nil {
		t.Fatal("expected grounded question validation to fail")
	}

	var invalid invalidGroundedQuestionsError
	if !errors.As(err, &invalid) {
		t.Fatalf("expected invalid grounded questions error, got %T", err)
	}
	if len(invalid.Facts) != 1 || invalid.Facts[0].Category != "framework" {
		t.Fatalf("expected framework fact to be attached, got %+v", invalid)
	}
	if instruction := invalid.retryInstruction(); !strings.Contains(instruction, "Grounding facts to use instead:") {
		t.Fatalf("expected retry instruction to include grounding facts, got:\n%s", instruction)
	}
}

func assertGroundingFact(t *testing.T, grounding model.RepoGrounding, category, contains string) {
	t.Helper()
	for _, fact := range grounding.Facts {
		if fact.Category == category && strings.Contains(fact.Value, contains) {
			return
		}
	}
	t.Fatalf("expected grounding fact category %q to contain %q, got %+v", category, contains, grounding.Facts)
}

func TestEnsureRepoGroundingReusesMatchingReadySnapshot(t *testing.T) {
	tempDir := t.TempDir()
	ready := model.RepoGrounding{
		Status:        model.RepoGroundingReady,
		WorkspaceRoot: filepath.Clean(tempDir),
		Summary:       "ready",
		Facts: []model.GroundingFact{
			{Category: "framework", Label: "Frameworks", Value: "Bubble Tea", EvidencePaths: []string{"go.mod"}},
		},
		Unknowns:     []string{},
		ScannedFiles: []string{"go.mod"},
	}

	reused, err := ensureRepoGrounding(tempDir, ready)
	if err != nil {
		t.Fatalf("ensure repo grounding: %v", err)
	}
	if reused.Summary != "ready" {
		t.Fatalf("expected existing grounding to be reused, got %+v", reused)
	}
}
