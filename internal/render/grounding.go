package render

import (
	"fmt"
	"strings"

	"panelofexperts/internal/model"
)

func RenderRepoGroundingMarkdown(grounding model.RepoGrounding) string {
	var b strings.Builder

	title := "Repo Grounding"
	if grounding.WorkspaceRoot != "" {
		fmt.Fprintf(&b, "# %s\n\n", title)
		fmt.Fprintf(&b, "Workspace: `%s`\n\n", grounding.WorkspaceRoot)
	} else {
		fmt.Fprintf(&b, "# %s\n\n", title)
	}

	if summary := strings.TrimSpace(grounding.Summary); summary != "" {
		fmt.Fprintf(&b, "Status: **%s**\n\n", grounding.Status)
		writeSection(&b, "Summary", summary)
	}

	if len(grounding.Facts) > 0 {
		b.WriteString("## Facts\n\n")
		for _, fact := range grounding.Facts {
			if strings.TrimSpace(fact.Value) == "" {
				continue
			}
			fmt.Fprintf(&b, "- **%s**: %s", fact.Label, fact.Value)
			if len(fact.EvidencePaths) > 0 {
				fmt.Fprintf(&b, " (`%s`)", strings.Join(fact.EvidencePaths, "`, `"))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	writeListSection(&b, "Unknowns", grounding.Unknowns)

	if len(grounding.ScannedFiles) > 0 {
		b.WriteString("## Scanned Files\n\n")
		for _, path := range grounding.ScannedFiles {
			fmt.Fprintf(&b, "- `%s`\n", path)
		}
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String()) + "\n"
}
