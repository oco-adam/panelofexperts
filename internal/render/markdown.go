package render

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	writeSection(&b, "Change Summary", proposal.ChangeSummary)
	b.WriteString("## Metadata\n\n")
	fmt.Fprintf(&b, "- Run ID: `%s`\n", run.ID)
	fmt.Fprintf(&b, "- Status: `%s`\n", run.Status)
	fmt.Fprintf(&b, "- Stop reason: `%s`\n", run.StopReason)
	fmt.Fprintf(&b, "- Rounds completed: `%d`\n", len(run.Rounds))
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
