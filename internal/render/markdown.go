package render

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"panelofexperts/internal/model"
)

func RenderBriefMarkdown(brief model.Brief) string {
	var b strings.Builder
	title := brief.ProjectTitle
	if title == "" {
		title = "Untitled project"
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	writeSection(&b, "Intent Summary", brief.IntentSummary)
	if brief.TaskKind != "" {
		fmt.Fprintf(&b, "Task kind: **%s**\n\n", brief.TaskKind)
	}
	if strings.TrimSpace(brief.TargetFilePath) != "" {
		fmt.Fprintf(&b, "Target file: `%s`\n\n", brief.TargetFilePath)
	}
	writeListSection(&b, "Goals", brief.Goals)
	writeListSection(&b, "Constraints", brief.Constraints)
	writeListSection(&b, "Open Questions", brief.OpenQuestions)
	writeSection(&b, "Manager Notes", brief.ManagerNotes)
	fmt.Fprintf(&b, "Ready to start: **%t**\n", brief.ReadyToStart)
	return strings.TrimSpace(b.String()) + "\n"
}

func RenderProposalMarkdown(proposal model.Proposal, run model.RunState) string {
	var b strings.Builder
	title := proposal.Title
	if title == "" {
		title = run.ProjectTitle
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	writeSection(&b, "Context", proposal.Context)
	writeListSection(&b, "Goals", proposal.Goals)
	writeListSection(&b, "Constraints", proposal.Constraints)
	if len(proposal.RecommendedPlan) > 0 {
		b.WriteString("## Recommended Plan\n\n")
		for i, item := range proposal.RecommendedPlan {
			fmt.Fprintf(&b, "%d. **%s**", i+1, strings.TrimSpace(item.Title))
			details := strings.TrimSpace(item.Details)
			if details != "" {
				fmt.Fprintf(&b, ": %s", details)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	writeListSection(&b, "Risks", proposal.Risks)
	writeListSection(&b, "Open Questions", proposal.OpenQuestions)
	writeListSection(&b, "Consensus Notes", proposal.ConsensusNotes)
	if strings.TrimSpace(proposal.DeliverablePath) != "" {
		b.WriteString("## Deliverable\n\n")
		fmt.Fprintf(&b, "- Path: `%s`\n\n", proposal.DeliverablePath)
	}
	writeSection(&b, "Change Summary", proposal.ChangeSummary)
	b.WriteString("## Metadata\n\n")
	fmt.Fprintf(&b, "- Run ID: `%s`\n", run.ID)
	fmt.Fprintf(&b, "- Status: `%s`\n", run.Status)
	fmt.Fprintf(&b, "- Stop reason: `%s`\n", run.StopReason)
	fmt.Fprintf(&b, "- Rounds completed: `%d`\n", len(run.Rounds))
	return strings.TrimSpace(b.String()) + "\n"
}

func RenderDeliverableMarkdown(run model.RunState, proposal model.Proposal) string {
	if strings.TrimSpace(proposal.DeliverableMarkdown) != "" {
		return strings.TrimSpace(proposal.DeliverableMarkdown) + "\n"
	}

	var b strings.Builder
	title := deliverableTitle(run, proposal)
	fmt.Fprintf(&b, "# %s\n\n", title)

	intro := deliverableIntro(run, proposal)
	if intro != "" {
		b.WriteString(intro)
		b.WriteString("\n\n")
	}

	goalsTitle := "Goals"
	if looksLikeDesignSystem(run, proposal) {
		goalsTitle = "Design Goals"
	}
	writeListSection(&b, goalsTitle, proposal.Goals)

	constraints := filterDocumentConstraints(proposal.Constraints)
	writeListSection(&b, "Constraints", constraints)

	for _, item := range proposal.RecommendedPlan {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			continue
		}
		writeSection(&b, title, sanitizeDeliverableText(item.Details))
	}

	writeListSection(&b, "Risks", proposal.Risks)
	writeListSection(&b, "Consensus Notes", proposal.ConsensusNotes)
	writeListSection(&b, "Open Questions", proposal.OpenQuestions)
	return strings.TrimSpace(b.String()) + "\n"
}

func ProposalHash(proposal model.Proposal) string {
	data, _ := json.Marshal(proposal)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writeSection(b *strings.Builder, title, body string) {
	body = strings.TrimSpace(body)
	if body == "" {
		return
	}
	fmt.Fprintf(b, "## %s\n\n%s\n\n", title, body)
}

func writeListSection(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "## %s\n\n", title)
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		fmt.Fprintf(b, "- %s\n", item)
	}
	b.WriteString("\n")
}

func looksLikeDesignSystem(run model.RunState, proposal model.Proposal) bool {
	base := strings.TrimSpace(strings.TrimSuffix(filepath.Base(run.Brief.TargetFilePath), filepath.Ext(run.Brief.TargetFilePath)))
	if strings.EqualFold(base, "design") {
		return true
	}
	title := strings.ToLower(strings.TrimSpace(proposal.Title + " " + run.Brief.IntentSummary))
	return strings.Contains(title, "design system")
}

func deliverableTitle(run model.RunState, proposal model.Proposal) string {
	base := strings.TrimSpace(strings.TrimSuffix(filepath.Base(run.Brief.TargetFilePath), filepath.Ext(run.Brief.TargetFilePath)))
	if strings.EqualFold(base, "design") {
		project := strings.TrimSpace(run.ProjectTitle)
		if project == "" {
			project = "Application"
		}
		return fmt.Sprintf("%s TUI Design System", project)
	}
	if base != "" {
		return humanizeIdentifier(base)
	}
	if strings.TrimSpace(proposal.Title) != "" {
		return strings.TrimSpace(proposal.Title)
	}
	if strings.TrimSpace(run.ProjectTitle) != "" {
		return strings.TrimSpace(run.ProjectTitle)
	}
	return "Deliverable"
}

func deliverableIntro(run model.RunState, proposal model.Proposal) string {
	if looksLikeDesignSystem(run, proposal) {
		project := strings.TrimSpace(run.ProjectTitle)
		if project == "" {
			project = "this application"
		}
		return fmt.Sprintf(
			"This document defines the target-state design system for the %s terminal UI. It is the canonical reference for layout, semantic styling, interaction behavior, accessibility rules, and future UI extension.",
			project,
		)
	}
	context := sanitizeDeliverableText(proposal.Context)
	if context != "" {
		return context
	}
	return "This document captures the final agreed deliverable for the current discussion."
}

func sanitizeDeliverableText(input string) string {
	replacer := strings.NewReplacer(
		"Planning-only proposal for ", "",
		"Planning-only proposal", "Proposal",
		"This iteration preserves the target-state design-system approach for the TUI and keeps", "This document uses a target-state design-system approach and keeps",
		"once repository grounding is reviewed in a later step", "as implementation details are refined",
		"Once repository grounding is reviewed in a later step, ", "",
		"Stay in planning mode for this step; do not inspect repository files or edit files yet.", "",
		"when drafting begins", "during implementation",
	)
	return strings.TrimSpace(replacer.Replace(strings.TrimSpace(input)))
}

func filterDocumentConstraints(items []string) []string {
	filtered := make([]string, 0, len(items))
	for _, item := range items {
		item = sanitizeDeliverableText(item)
		lower := strings.ToLower(item)
		if item == "" {
			continue
		}
		if strings.Contains(lower, "planning mode") || strings.Contains(lower, "edit files yet") {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func humanizeIdentifier(input string) string {
	input = strings.NewReplacer("-", " ", "_", " ").Replace(strings.TrimSpace(input))
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return ""
	}
	for i, part := range parts {
		lower := strings.ToLower(part)
		parts[i] = strings.ToUpper(lower[:1]) + lower[1:]
	}
	return strings.Join(parts, " ")
}
