package orchestrator

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"panelofexperts/internal/model"
)

const groundingReadLimit = 64 * 1024

var ignoredGroundingDirs = map[string]struct{}{
	".git":              {},
	".panel-of-experts": {},
	"node_modules":      {},
	"vendor":            {},
	"dist":              {},
	"build":             {},
	"out":               {},
}

type invalidGroundedQuestionsError struct {
	Questions  []string
	Categories []string
	Facts      []model.GroundingFact
}

func (e invalidGroundedQuestionsError) Error() string {
	questions := append([]string{}, e.Questions...)
	sort.Strings(questions)
	return fmt.Sprintf("manager asked repo-answerable question(s): %s", strings.Join(questions, "; "))
}

func (e invalidGroundedQuestionsError) retryInstruction() string {
	lines := []string{
		"Your previous brief asked the user for repo facts that are already covered by repo grounding.",
		"Remove or replace those questions. Only ask about user intent, preferences, constraints, or tradeoffs that grounding cannot resolve.",
	}
	if len(e.Questions) > 0 {
		lines = append(lines, "", "Questions to remove or replace:")
		for _, question := range e.Questions {
			lines = append(lines, "- "+question)
		}
	}
	if len(e.Facts) > 0 {
		lines = append(lines, "", "Grounding facts to use instead:")
		for _, fact := range e.Facts {
			line := fmt.Sprintf("- %s: %s", fact.Label, fact.Value)
			if len(fact.EvidencePaths) > 0 {
				line += fmt.Sprintf(" (%s)", strings.Join(fact.EvidencePaths, ", "))
			}
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func ensureRepoGrounding(cwd string, existing model.RepoGrounding) (model.RepoGrounding, error) {
	if existing.Status == model.RepoGroundingReady &&
		filepath.Clean(strings.TrimSpace(existing.WorkspaceRoot)) == filepath.Clean(strings.TrimSpace(cwd)) &&
		len(existing.Facts) > 0 {
		return normalizeGrounding(existing, cwd), nil
	}
	grounding, err := collectRepoGrounding(cwd)
	if err != nil {
		return model.RepoGrounding{
			Status:        model.RepoGroundingFailed,
			WorkspaceRoot: cwd,
			Summary:       fmt.Sprintf("Repo grounding failed: %v", err),
			Facts:         []model.GroundingFact{},
			Unknowns:      []string{"Unable to produce the required repo grounding snapshot."},
			ScannedFiles:  []string{},
		}, err
	}
	return grounding, nil
}

func collectRepoGrounding(cwd string) (model.RepoGrounding, error) {
	root, err := filepath.Abs(cwd)
	if err != nil {
		return model.RepoGrounding{}, err
	}

	var facts []model.GroundingFact
	scanned := map[string]struct{}{}
	unknowns := []string{}

	addFact := func(category, label, value string, evidence ...string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		normalizedEvidence := uniqueSortedPaths(evidence)
		for _, path := range normalizedEvidence {
			scanned[path] = struct{}{}
		}
		facts = append(facts, model.GroundingFact{
			Category:      category,
			Label:         label,
			Value:         value,
			EvidencePaths: normalizedEvidence,
		})
	}

	manifestPaths, err := findMatchingFiles(root, func(path string, d fs.DirEntry) bool {
		if d.IsDir() {
			return false
		}
		switch filepath.Base(path) {
		case "go.mod", "package.json", "pyproject.toml", "Cargo.toml", "dune-project", "pnpm-workspace.yaml", "tsconfig.json", "tsconfig.base.json":
			return true
		}
		return strings.HasSuffix(path, ".opam")
	})
	if err != nil {
		return model.RepoGrounding{}, err
	}
	representativeManifests := representativeManifestPaths(manifestPaths)
	runtimes := detectedRuntimes(manifestPaths)
	if len(runtimes) > 0 {
		label := "Primary Runtime"
		if len(runtimes) > 1 {
			label = "Workspace Runtimes"
		}
		addFact("language_runtime", label, strings.Join(runtimes, ", "), representativeManifests...)
	}
	if len(representativeManifests) > 0 {
		addFact("manifest", "Detected Manifests", joinCodePaths(representativeManifests), representativeManifests...)
	}
	projectLayout := describeWorkspaceProjects(representativeManifests)
	if len(projectLayout) > 0 {
		addFact("project_layout", "Workspace Projects", strings.Join(projectLayout, "; "), representativeManifests...)
	}
	if len(representativeManifests) == 0 {
		unknowns = append(unknowns, "No high-signal package manifest was detected at the workspace root.")
	}

	if goMod, rel, err := readRelFile(root, "go.mod"); err == nil {
		var frameworks []string
		if strings.Contains(goMod, "charm.land/bubbletea") {
			frameworks = append(frameworks, "Bubble Tea")
		}
		if strings.Contains(goMod, "charm.land/bubbles") {
			frameworks = append(frameworks, "Bubbles")
		}
		if strings.Contains(goMod, "charm.land/lipgloss") {
			frameworks = append(frameworks, "Lip Gloss")
		}
		if len(frameworks) > 0 {
			addFact("framework", "Frameworks", strings.Join(frameworks, ", "), rel)
		}
	}

	if containsString(runtimes, "Go") {
		entrypoints, err := findMatchingFiles(root, func(path string, d fs.DirEntry) bool {
			if d.IsDir() {
				return false
			}
			return strings.HasPrefix(path, "cmd/") && strings.HasSuffix(path, "/main.go")
		})
		if err != nil {
			return model.RepoGrounding{}, err
		}
		if len(entrypoints) > 0 {
			values := make([]string, 0, len(entrypoints))
			for _, path := range entrypoints {
				values = append(values, fmt.Sprintf("`%s`", strings.TrimSuffix(path, "/main.go")))
			}
			addFact("entrypoint", "CLI Entrypoints", strings.Join(values, ", "), entrypoints...)
		} else {
			unknowns = append(unknowns, "No CLI entrypoint was detected under cmd/.")
		}
	}

	docCandidates := []string{}
	for _, rel := range []string{"README.md", "README", "DESIGN.md", "NORTH_STAR.md", "spec/public-contract/README.md"} {
		if path, ok := existingRelPath(root, rel); ok {
			docCandidates = append(docCandidates, path)
		}
	}
	docFiles, err := findMatchingFiles(root, func(path string, d fs.DirEntry) bool {
		if d.IsDir() {
			return false
		}
		return strings.HasPrefix(path, "docs/") && strings.HasSuffix(path, ".md")
	})
	if err != nil {
		return model.RepoGrounding{}, err
	}
	docCandidates = append(docCandidates, docFiles...)
	if len(docCandidates) > 0 {
		docCandidates = uniqueSortedPaths(docCandidates)
		addFact("docs", "Key Docs", joinCodePaths(docCandidates), docCandidates...)
	} else {
		unknowns = append(unknowns, "No high-signal README or docs markdown files were detected.")
	}

	testFiles, err := findMatchingFiles(root, func(path string, d fs.DirEntry) bool {
		if d.IsDir() {
			return false
		}
		return strings.HasSuffix(path, "_test.go")
	})
	if err != nil {
		return model.RepoGrounding{}, err
	}
	if len(testFiles) > 0 {
		value := fmt.Sprintf("%d Go test file(s) detected", len(testFiles))
		addFact("tests", "Tests", value, limitPaths(testFiles, 6)...)
	}

	releaseEvidence := []string{}
	workflowFiles, err := findMatchingFiles(root, func(path string, d fs.DirEntry) bool {
		if d.IsDir() {
			return false
		}
		return strings.HasPrefix(path, ".github/workflows/") &&
			(strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml"))
	})
	if err != nil {
		return model.RepoGrounding{}, err
	}
	releaseEvidence = append(releaseEvidence, workflowFiles...)
	if rel, ok := existingRelPath(root, ".goreleaser.yaml"); ok {
		releaseEvidence = append(releaseEvidence, rel)
	}
	if len(releaseEvidence) > 0 {
		releaseEvidence = uniqueSortedPaths(releaseEvidence)
		releaseParts := []string{}
		if len(workflowFiles) > 0 {
			releaseParts = append(releaseParts, "GitHub Actions")
		}
		for _, path := range releaseEvidence {
			if strings.HasSuffix(path, ".goreleaser.yaml") {
				releaseParts = append(releaseParts, "GoReleaser")
				break
			}
		}
		addFact("release_tooling", "Release Tooling", strings.Join(uniqueStrings(releaseParts), ", "), releaseEvidence...)
	} else {
		unknowns = append(unknowns, "No release automation file was detected in the high-signal scan.")
	}

	grounding := model.RepoGrounding{
		Status:        model.RepoGroundingReady,
		WorkspaceRoot: root,
		Summary:       summarizeGrounding(facts),
		Facts:         facts,
		Unknowns:      uniqueStrings(unknowns),
		ScannedFiles:  uniqueSortedPaths(mapKeys(scanned)),
	}
	return normalizeGrounding(grounding, root), nil
}

func normalizeGrounding(grounding model.RepoGrounding, cwd string) model.RepoGrounding {
	grounding.WorkspaceRoot = strings.TrimSpace(grounding.WorkspaceRoot)
	if grounding.WorkspaceRoot == "" {
		grounding.WorkspaceRoot = cwd
	}
	if grounding.Status == "" {
		grounding.Status = model.RepoGroundingPending
	}
	if len(grounding.Facts) == 0 {
		grounding.Facts = []model.GroundingFact{}
	}
	if len(grounding.Unknowns) == 0 {
		grounding.Unknowns = []string{}
	}
	if len(grounding.ScannedFiles) == 0 {
		grounding.ScannedFiles = []string{}
	}
	for i := range grounding.Facts {
		grounding.Facts[i].EvidencePaths = uniqueSortedPaths(grounding.Facts[i].EvidencePaths)
	}
	grounding.Unknowns = uniqueStrings(grounding.Unknowns)
	grounding.ScannedFiles = uniqueSortedPaths(grounding.ScannedFiles)
	return grounding
}

func summarizeGrounding(facts []model.GroundingFact) string {
	lookup := map[string]string{}
	for _, fact := range facts {
		if _, ok := lookup[fact.Category]; ok {
			continue
		}
		lookup[fact.Category] = fact.Value
	}

	parts := []string{}
	if runtime := lookup["language_runtime"]; runtime != "" {
		parts = append(parts, runtime+" workspace")
	}
	if layout := lookup["project_layout"]; layout != "" {
		parts = append(parts, "projects: "+layout)
	}
	if framework := lookup["framework"]; framework != "" {
		parts = append(parts, "using "+framework)
	}
	if entry := lookup["entrypoint"]; entry != "" {
		parts = append(parts, "with entrypoints "+entry)
	}
	if docs := lookup["docs"]; docs != "" {
		parts = append(parts, "docs in "+docs)
	}
	if _, ok := lookup["tests"]; ok {
		parts = append(parts, "tests present")
	}
	if release := lookup["release_tooling"]; release != "" {
		parts = append(parts, "release automation via "+release)
	}
	if len(parts) == 0 {
		return "High-signal scan completed, but it did not yield enough repo facts to summarize the workspace."
	}
	return strings.TrimSuffix(strings.Join(parts, ", ")+".", ",.")
}

func validateGroundedQuestions(brief model.Brief, grounding model.RepoGrounding) error {
	if grounding.Status != model.RepoGroundingReady || len(brief.OpenQuestions) == 0 {
		return nil
	}

	knownCategories := map[string]struct{}{}
	for _, fact := range grounding.Facts {
		if strings.TrimSpace(fact.Value) == "" {
			continue
		}
		knownCategories[fact.Category] = struct{}{}
	}

	invalidQuestions := []string{}
	invalidCategories := []string{}
	for _, question := range brief.OpenQuestions {
		category := groundedQuestionCategory(question, knownCategories)
		if category == "" {
			continue
		}
		invalidQuestions = append(invalidQuestions, strings.TrimSpace(question))
		invalidCategories = append(invalidCategories, category)
	}
	if len(invalidQuestions) == 0 {
		return nil
	}
	return invalidGroundedQuestionsError{
		Questions:  uniqueStrings(invalidQuestions),
		Categories: uniqueStrings(invalidCategories),
		Facts:      groundingFactsForCategories(grounding, invalidCategories),
	}
}

func groundingFactsForCategories(grounding model.RepoGrounding, categories []string) []model.GroundingFact {
	want := map[string]struct{}{}
	for _, category := range categories {
		want[category] = struct{}{}
	}
	facts := make([]model.GroundingFact, 0, len(categories))
	for _, fact := range grounding.Facts {
		if _, ok := want[fact.Category]; ok {
			facts = append(facts, fact)
		}
	}
	return facts
}

func groundedQuestionCategory(question string, known map[string]struct{}) string {
	lower := strings.ToLower(strings.TrimSpace(question))
	if lower == "" {
		return ""
	}

	rules := []struct {
		category string
		keywords []string
	}{
		{category: "framework", keywords: []string{"framework", "bubble tea", "bubbletea", "renderer", "rendering approach", "lip gloss", "lipgloss", "tcell"}},
		{category: "language_runtime", keywords: []string{"what language", "which language", "runtime", "tech stack", "go module", "implemented in", "ocaml", "rust", "typescript", "javascript"}},
		{category: "entrypoint", keywords: []string{"entrypoint", "entry point", "main.go", "main package", "where does the app start", "starting point"}},
		{category: "docs", keywords: []string{"readme", "design.md", "documentation", "docs folder", "existing docs"}},
		{category: "tests", keywords: []string{"tests exist", "test suite", "unit tests", "integration tests", "coverage", "_test.go"}},
		{category: "release_tooling", keywords: []string{"github actions", "goreleaser", "release workflow", "release automation", "ci pipeline", "workflow file"}},
	}

	for _, rule := range rules {
		if _, ok := known[rule.category]; !ok {
			continue
		}
		for _, keyword := range rule.keywords {
			if strings.Contains(lower, keyword) {
				return rule.category
			}
		}
	}
	return ""
}

func existingRelPath(root, rel string) (string, bool) {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if _, err := os.Stat(abs); err != nil {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func readRelFile(root, rel string) (string, string, error) {
	rel = filepath.ToSlash(rel)
	path := filepath.Join(root, filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	if len(data) > groundingReadLimit {
		data = data[:groundingReadLimit]
	}
	return string(data), rel, nil
}

func findMatchingFiles(root string, match func(path string, d fs.DirEntry) bool) ([]string, error) {
	matches := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if _, ok := ignoredGroundingDirs[d.Name()]; ok {
				return filepath.SkipDir
			}
			return nil
		}
		if match(rel, d) {
			matches = append(matches, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

func joinCodePaths(paths []string) string {
	paths = uniqueSortedPaths(paths)
	quoted := make([]string, 0, len(paths))
	for _, path := range paths {
		quoted = append(quoted, fmt.Sprintf("`%s`", path))
	}
	return strings.Join(quoted, ", ")
}

func uniqueSortedPaths(paths []string) []string {
	return uniqueStrings(paths)
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = filepath.ToSlash(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func mapKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	return keys
}

func limitPaths(paths []string, limit int) []string {
	paths = uniqueSortedPaths(paths)
	if len(paths) <= limit {
		return paths
	}
	return paths[:limit]
}

func detectedRuntimes(manifests []string) []string {
	seen := map[string]struct{}{}
	for _, path := range manifests {
		switch base := filepath.Base(path); {
		case base == "go.mod":
			seen["Go"] = struct{}{}
		case base == "package.json" || strings.HasPrefix(base, "tsconfig"):
			seen["JavaScript/TypeScript"] = struct{}{}
		case base == "pyproject.toml":
			seen["Python"] = struct{}{}
		case base == "Cargo.toml":
			seen["Rust"] = struct{}{}
		case base == "dune-project" || strings.HasSuffix(path, ".opam"):
			seen["OCaml"] = struct{}{}
		}
	}
	order := []string{"Go", "OCaml", "Rust", "JavaScript/TypeScript", "Python"}
	runtimes := make([]string, 0, len(seen))
	for _, runtime := range order {
		if _, ok := seen[runtime]; ok {
			runtimes = append(runtimes, runtime)
		}
	}
	for runtime := range seen {
		if !containsString(runtimes, runtime) {
			runtimes = append(runtimes, runtime)
		}
	}
	return runtimes
}

func representativeManifestPaths(manifests []string) []string {
	manifests = uniqueSortedPaths(manifests)
	if len(manifests) == 0 {
		return []string{}
	}

	chosen := []string{}
	groupBest := map[string]string{}
	for _, path := range manifests {
		if strings.Count(path, "/") == 0 {
			chosen = append(chosen, path)
			continue
		}
		group := strings.SplitN(path, "/", 2)[0]
		best, ok := groupBest[group]
		if !ok || manifestPreference(path, best) < 0 {
			groupBest[group] = path
		}
	}
	groups := make([]string, 0, len(groupBest))
	for group := range groupBest {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	for _, group := range groups {
		chosen = append(chosen, groupBest[group])
	}

	sort.SliceStable(chosen, func(i, j int) bool {
		left, right := chosen[i], chosen[j]
		leftDepth, rightDepth := strings.Count(left, "/"), strings.Count(right, "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return left < right
	})
	return limitPaths(chosen, 6)
}

func describeWorkspaceProjects(manifests []string) []string {
	descriptions := make([]string, 0, len(manifests))
	for _, path := range manifests {
		runtime := manifestRuntime(path)
		if runtime == "" {
			continue
		}
		if strings.Count(path, "/") == 0 {
			descriptions = append(descriptions, fmt.Sprintf("%s root (`%s`)", runtime, path))
			continue
		}
		label := "project"
		switch filepath.Base(path) {
		case "Cargo.toml":
			label = "workspace"
		case "package.json", "tsconfig.json", "tsconfig.base.json":
			label = "app"
		}
		descriptions = append(descriptions, fmt.Sprintf("%s %s (`%s`)", runtime, label, path))
	}
	return uniqueStrings(descriptions)
}

func manifestRuntime(path string) string {
	switch base := filepath.Base(path); {
	case base == "go.mod":
		return "Go"
	case base == "package.json" || strings.HasPrefix(base, "tsconfig"):
		return "JavaScript/TypeScript"
	case base == "pyproject.toml":
		return "Python"
	case base == "Cargo.toml":
		return "Rust"
	case base == "dune-project" || strings.HasSuffix(path, ".opam"):
		return "OCaml"
	default:
		return ""
	}
}

func manifestPreference(candidate, current string) int {
	candidateDepth, currentDepth := strings.Count(candidate, "/"), strings.Count(current, "/")
	if candidateDepth != currentDepth {
		return candidateDepth - currentDepth
	}
	candidateRank, currentRank := manifestRank(candidate), manifestRank(current)
	if candidateRank != currentRank {
		return candidateRank - currentRank
	}
	if candidate < current {
		return -1
	}
	if candidate > current {
		return 1
	}
	return 0
}

func manifestRank(path string) int {
	switch filepath.Base(path) {
	case "go.mod", "dune-project", "Cargo.toml", "package.json", "pyproject.toml":
		return 0
	case "tsconfig.json", "tsconfig.base.json":
		return 1
	default:
		if strings.HasSuffix(path, ".opam") {
			return 2
		}
		return 3
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
