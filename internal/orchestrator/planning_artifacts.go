package orchestrator

import (
	"strings"

	"panelofexperts/internal/model"
)

var briefPlanningArtifactSnippets = []string{
	"stay in planning mode",
	"return only json",
	"repo grounding",
	"manager updated the brief",
	"do not attempt to exit planning mode",
	"do not call any write/edit/create tool",
	"do not call any write, edit, or create tool",
}

var documentPlanningArtifactSnippets = []string{
	"stay in planning mode",
	"return only json",
	"repo grounding",
	"expert review",
	"planning proposal",
	"manager updated the brief",
	"do not edit files",
	"do not call any write/edit/create tool",
	"do not call any write, edit, or create tool",
}

func sanitizeBriefForStorage(brief model.Brief) model.Brief {
	brief.Constraints = sanitizeStringItems(brief.Constraints, briefPlanningArtifactSnippets)
	brief.ManagerNotes = sanitizeTextUnits(brief.ManagerNotes, briefPlanningArtifactSnippets)
	return brief
}

func briefForDocumentPrompt(brief model.Brief) model.Brief {
	sanitized := brief
	sanitized.Constraints = sanitizeStringItems(sanitized.Constraints, documentPlanningArtifactSnippets)
	sanitized.ManagerNotes = sanitizeTextUnits(sanitized.ManagerNotes, documentPlanningArtifactSnippets)
	return sanitized
}

func firstDocumentPlanningArtifact(text string) (string, bool) {
	return firstMatchingSnippet(text, documentPlanningArtifactSnippets)
}

func isPlanningArtifactError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "planning artifact")
}

func planningArtifactRetrySnippet(err error) string {
	if err == nil {
		return ""
	}
	snippet, _ := firstMatchingSnippet(err.Error(), documentPlanningArtifactSnippets)
	return snippet
}

func sanitizeStringItems(items []string, snippets []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	filtered := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, matched := firstMatchingSnippet(item, snippets); matched {
			continue
		}
		filtered = append(filtered, item)
	}
	if len(filtered) == 0 {
		return []string{}
	}
	return filtered
}

func sanitizeTextUnits(text string, snippets []string) string {
	units := splitTextUnits(text)
	if len(units) == 0 {
		return ""
	}
	filtered := make([]string, 0, len(units))
	for _, unit := range units {
		if _, matched := firstMatchingSnippet(unit, snippets); matched {
			continue
		}
		filtered = append(filtered, unit)
	}
	return strings.TrimSpace(strings.Join(filtered, " "))
}

func splitTextUnits(text string) []string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	units := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ". ")
		for i, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if i < len(parts)-1 && !strings.HasSuffix(part, ".") {
				part += "."
			}
			units = append(units, part)
		}
	}
	return units
}

func firstMatchingSnippet(text string, snippets []string) (string, bool) {
	lower := strings.ToLower(text)
	for _, snippet := range snippets {
		if strings.Contains(lower, snippet) {
			return snippet, true
		}
	}
	return "", false
}
