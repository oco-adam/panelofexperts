package orchestrator

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"panelofexperts/internal/model"
)

var markdownPathPattern = regexp.MustCompile(`(?i)([A-Za-z0-9_./-]+\.(?:md|markdown))`)

const briefSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["project_title", "intent_summary", "task_kind", "target_file_path", "goals", "constraints", "ready_to_start", "open_questions", "manager_notes"],
  "properties": {
    "project_title": {"type": "string"},
    "intent_summary": {"type": "string"},
    "task_kind": {"type": "string"},
    "target_file_path": {"type": "string"},
    "goals": {"type": "array", "items": {"type": "string"}},
    "constraints": {"type": "array", "items": {"type": "string"}},
    "ready_to_start": {"type": "boolean"},
    "open_questions": {"type": "array", "items": {"type": "string"}},
    "manager_notes": {"type": "string"}
  }
}`

const proposalSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["title", "context", "goals", "constraints", "recommended_plan", "risks", "open_questions", "consensus_notes", "converged", "change_summary"],
  "properties": {
    "title": {"type": "string"},
    "context": {"type": "string"},
    "goals": {"type": "array", "items": {"type": "string"}},
    "constraints": {"type": "array", "items": {"type": "string"}},
    "recommended_plan": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["title", "details"],
        "properties": {
          "title": {"type": "string"},
          "details": {"type": "string"}
        }
      }
    },
    "risks": {"type": "array", "items": {"type": "string"}},
    "open_questions": {"type": "array", "items": {"type": "string"}},
    "consensus_notes": {"type": "array", "items": {"type": "string"}},
    "converged": {"type": "boolean"},
    "change_summary": {"type": "string"}
  }
}`

const documentDraftSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["path", "markdown"],
  "properties": {
    "path": {"type": "string"},
    "markdown": {"type": "string"}
  }
}`

const documentVersionSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["path", "markdown", "change_summary", "converged"],
  "properties": {
    "path": {"type": "string"},
    "markdown": {"type": "string"},
    "change_summary": {"type": "string"},
    "converged": {"type": "boolean"}
  }
}`

const reviewSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["lens", "summary", "strengths", "concerns", "recommendations", "blocking_risks", "requires_changes"],
  "properties": {
    "lens": {"type": "string"},
    "summary": {"type": "string"},
    "strengths": {"type": "array", "items": {"type": "string"}},
    "concerns": {"type": "array", "items": {"type": "string"}},
    "recommendations": {"type": "array", "items": {"type": "string"}},
    "blocking_risks": {"type": "array", "items": {"type": "string"}},
    "requires_changes": {"type": "boolean"}
  }
}`

func buildBriefPrompt(run model.RunState, userMessage string) string {
	hint := inferTaskHint(run, userMessage)
	return strings.TrimSpace(fmt.Sprintf(`
Return only JSON for a planning brief.

Stay in planning mode. Repo grounding has already been collected for %s and must be treated as the baseline workspace context.
Use repo grounding first. Inspect repository files only when the grounding leaves a real gap, and do not edit files.
Do not ask the user for repo facts already covered by repo grounding or obvious high-signal files. Only ask about intent, preferences, constraints, or tradeoffs the repo cannot answer.
Do not attempt to exit planning mode or call any write/edit/create tool.
Use the latest user request and any existing brief context to set:
- project_title
- intent_summary
- task_kind
- target_file_path
- goals
- constraints
- ready_to_start
- open_questions
- manager_notes

Keep the title stable once it is clear. Set ready_to_start to true only when the brief is actionable enough for expert review.
If the request clearly targets a markdown file deliverable, set task_kind to "document" and target_file_path to that file.
Otherwise set task_kind to "plan" and target_file_path to an empty string.

Target repository path for the later discussion: %s
Task kind hint from the app: %s
Target file hint from the app: %s
Repo grounding: %s
Existing brief: %s
Previous manager turns: %s
Latest user request: %s
	`, run.CWD, run.CWD, hint.Kind, hint.TargetFilePath, mustCompactJSON(run.RepoGrounding), mustCompactJSON(run.Brief), mustCompactJSON(run.ManagerTurns), strings.TrimSpace(userMessage)))
}

func buildInitialProposalPrompt(run model.RunState) string {
	return strings.TrimSpace(fmt.Sprintf(`
Return only JSON for an initial planning proposal.

Stay in planning mode. Repo grounding has already been collected for %s and must be treated as the baseline workspace context.
Use repo grounding first. Inspect repository files only when the grounding leaves a real gap, and do not edit files.
Do not attempt to exit planning mode or call any write/edit/create tool.
Produce a concrete proposal that is specific enough for expert review.

Repo grounding: %s
Brief: %s
Expert panel: %s
`, run.CWD, mustCompactJSON(run.RepoGrounding), mustCompactJSON(run.Brief), mustCompactJSON(run.Experts)))
}

func buildInitialDocumentPrompt(run model.RunState) string {
	targetPath := strings.TrimSpace(run.Brief.TargetFilePath)
	documentBrief := briefForDocumentPrompt(run.Brief)
	return strings.TrimSpace(fmt.Sprintf(`
Return only JSON for version 1 of the actual Markdown document.

You are writing the artifact itself, not a proposal about future edits.
Write the best complete document you can right now for %s.
Use repo grounding first. Inspect repository files read-only, including the existing target file and cited source files, when that improves accuracy or structure.
Do not call any write, edit, or create tool. The system will persist the returned document version.
Set:
- "path" to the target document path
- "markdown" to the full document content
- "change_summary" to what this version established or improved
- "converged" to true only if further expert review rounds are unlikely to materially improve the document

Write a complete, self-contained document. Do not return plans, TODOs, review notes, or narration about what you intend to do.
Do not copy orchestration instructions or internal phrases from the prompt or brief into the document. Never include phrases such as "repo grounding", "stay in planning mode", "return only JSON", "expert review", or "do not edit files" unless the target document genuinely requires those exact words.

Repo grounding: %s
Brief: %s
Expert panel: %s
`, targetPath, mustCompactJSON(run.RepoGrounding), mustCompactJSON(documentBrief), mustCompactJSON(run.Experts)))
}

func buildExpertReviewPrompt(run model.RunState, proposal model.Proposal, expert model.AgentConfig) string {
	lens := strings.ReplaceAll(string(expert.Lens), "_", " ")
	return strings.TrimSpace(fmt.Sprintf(`
Return only JSON for an expert review.

Stay in planning mode. Repo grounding has already been collected for %s and must be treated as the baseline workspace context.
Use repo grounding first. Inspect repository files only when the grounding leaves a real gap, and do not edit files.
Do not attempt to exit planning mode or call any write/edit/create tool.
Your review lens: %s
Review the current proposal critically but constructively. Focus on your lens and flag obvious high-risk issues.

Repo grounding: %s
Brief: %s
Current proposal: %s
`, run.CWD, lens, mustCompactJSON(run.RepoGrounding), mustCompactJSON(run.Brief), mustCompactJSON(proposal)))
}

func buildExpertDocumentReviewPrompt(run model.RunState, draft model.DocumentDraft, expert model.AgentConfig, version int) string {
	lens := strings.ReplaceAll(string(expert.Lens), "_", " ")
	documentBrief := briefForDocumentPrompt(run.Brief)
	return strings.TrimSpace(fmt.Sprintf(`
Return only JSON for an expert review.

You are reviewing version %d of the current document artifact itself.
Focus on the exact Markdown below, not on abstract future plans.
Your review lens: %s
Critique the document constructively and recommend concrete document changes. Set requires_changes to true only when the document should be revised before finalizing.
You may inspect repository files read-only when that helps verify claims or improve the review.
Do not call any write, edit, or create tool.

Repo grounding: %s
Brief: %s
Current document metadata: %s
Current document markdown:
%s
`, version, lens, mustCompactJSON(run.RepoGrounding), mustCompactJSON(documentBrief), mustCompactJSON(map[string]any{
		"path":           draft.Path,
		"change_summary": draft.ChangeSummary,
		"converged":      draft.Converged,
	}), strings.TrimSpace(draft.Markdown)))
}

func buildMergePrompt(run model.RunState, current model.Proposal, review model.ExpertReview, expert model.AgentConfig) string {
	return strings.TrimSpace(fmt.Sprintf(`
Return only JSON for an updated planning proposal.

Stay in planning mode. Repo grounding has already been collected for %s and must be treated as the baseline workspace context.
Use repo grounding first. Inspect repository files only when the grounding leaves a real gap, and do not edit files.
Do not attempt to exit planning mode or call any write/edit/create tool.
Consider exactly one expert review at a time. Incorporate useful feedback, reject weak feedback, and keep the proposal coherent.
If the review does not justify a change, you may return the proposal unchanged. Set converged to true only when the proposal is materially complete and stable.

Repo grounding: %s
Brief: %s
Current proposal: %s
Expert reviewer: %s
Expert review: %s
	`, run.CWD, mustCompactJSON(run.RepoGrounding), mustCompactJSON(run.Brief), mustCompactJSON(current), mustCompactJSON(map[string]any{
		"name": expert.Name,
		"lens": expert.Lens,
	}), mustCompactJSON(review)))
}

type reviewBundleItem struct {
	Name   string             `json:"name"`
	Lens   model.ExpertLens   `json:"lens"`
	Review model.ExpertReview `json:"review"`
}

func buildCombinedMergePrompt(run model.RunState, current model.Proposal, reviews []reviewBundleItem) string {
	return strings.TrimSpace(fmt.Sprintf(`
Return only JSON for an updated planning proposal.

Stay in planning mode. Repo grounding has already been collected for %s and must be treated as the baseline workspace context.
Use repo grounding first. Inspect repository files only when the grounding leaves a real gap, and do not edit files.
Do not attempt to exit planning mode or call any write/edit/create tool.
Consider the expert review bundle together. Reconcile conflicts, preserve strong feedback, reject weak or duplicative suggestions, and keep the proposal coherent.
If the review bundle does not justify a change, you may return the proposal unchanged. Set converged to true only when the proposal is materially complete and stable.

Repo grounding: %s
Brief: %s
Current proposal: %s
Expert review bundle: %s
`, run.CWD, mustCompactJSON(run.RepoGrounding), mustCompactJSON(run.Brief), mustCompactJSON(current), mustCompactJSON(reviews)))
}

func buildDocumentMergePrompt(run model.RunState, current model.DocumentDraft, review model.ExpertReview, expert model.AgentConfig, version int) string {
	documentBrief := briefForDocumentPrompt(run.Brief)
	return strings.TrimSpace(fmt.Sprintf(`
Return only JSON for the next full Markdown document version.

You are revising the actual document artifact, not writing a plan.
Consider exactly one expert review at a time. Incorporate useful feedback, reject weak feedback, and return the full updated document for version %d.
If the review does not justify changes, you may return the document unchanged.
Set "converged" to true only when further expert-review rounds are unlikely to materially improve the document.
Do not call any write, edit, or create tool.
Do not copy orchestration instructions or internal phrases from the prompt or brief into the document. Never include phrases such as "repo grounding", "stay in planning mode", "return only JSON", "expert review", or "do not edit files" unless the target document genuinely requires those exact words.

Repo grounding: %s
Brief: %s
Current document metadata: %s
Current document markdown:
%s
Expert reviewer: %s
Expert review: %s
`, version, mustCompactJSON(run.RepoGrounding), mustCompactJSON(documentBrief), mustCompactJSON(map[string]any{
		"path":           current.Path,
		"change_summary": current.ChangeSummary,
		"converged":      current.Converged,
	}), strings.TrimSpace(current.Markdown), mustCompactJSON(map[string]any{
		"name": expert.Name,
		"lens": expert.Lens,
	}), mustCompactJSON(review)))
}

func buildCombinedDocumentMergePrompt(run model.RunState, current model.DocumentDraft, reviews []reviewBundleItem, version int) string {
	documentBrief := briefForDocumentPrompt(run.Brief)
	return strings.TrimSpace(fmt.Sprintf(`
Return only JSON for the next full Markdown document version.

You are revising the actual document artifact, not writing a plan.
Consider the expert review bundle together. Reconcile conflicts, preserve strong feedback, reject weak or duplicative suggestions, and return the full updated document for version %d.
If the review bundle does not justify changes, you may return the document unchanged.
Set "converged" to true only when further expert-review rounds are unlikely to materially improve the document.
Do not call any write, edit, or create tool.
Do not copy orchestration instructions or internal phrases from the prompt or brief into the document. Never include phrases such as "repo grounding", "stay in planning mode", "return only JSON", "expert review", or "do not edit files" unless the target document genuinely requires those exact words.

Repo grounding: %s
Brief: %s
Current document metadata: %s
Current document markdown:
%s
Expert review bundle: %s
`, version, mustCompactJSON(run.RepoGrounding), mustCompactJSON(documentBrief), mustCompactJSON(map[string]any{
		"path":           current.Path,
		"change_summary": current.ChangeSummary,
		"converged":      current.Converged,
	}), strings.TrimSpace(current.Markdown), mustCompactJSON(reviews)))
}

func buildDocumentDraftPrompt(run model.RunState, proposal model.Proposal) string {
	targetPath := strings.TrimSpace(proposal.DeliverablePath)
	if targetPath == "" {
		targetPath = strings.TrimSpace(run.Brief.TargetFilePath)
	}
	documentBrief := briefForDocumentPrompt(run.Brief)
	return strings.TrimSpace(fmt.Sprintf(`
Return only JSON for the final Markdown deliverable.

You are drafting the actual contents that should be written to %s.
This is no longer a planning or proposal step. Write the finished document itself in the "markdown" field.
Use the agreed proposal as requirements, not as an output template.
You may inspect repository files read-only, including the current target file and cited source files, when that improves the final wording or structure.
Do not call any write, edit, or create tool. The system will write the returned markdown to disk.
If the target file already exists, rewrite it into a coherent replacement document rather than returning notes about what should change.
Do not return planning scaffolding, process narration, or review summaries. Do not say what you will do; do it in the markdown.
Unless the target document genuinely requires them, avoid proposal headings such as Goals, Constraints, Recommended Plan, Risks, Consensus Notes, Open Questions, and Change Summary.
Do not copy orchestration instructions or internal phrases from the prompt or brief into the document. Never include phrases such as "repo grounding", "stay in planning mode", "return only JSON", "expert review", or "do not edit files" unless the target document genuinely requires those exact words.
Set "path" to the target file path and "markdown" to the full final Markdown document.

Repo grounding: %s
Brief: %s
Final agreed proposal: %s
	`, targetPath, mustCompactJSON(run.RepoGrounding), mustCompactJSON(documentBrief), mustCompactJSON(proposal)))
}

func mustJSON(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

type taskHint struct {
	Kind           model.TaskKind
	TargetFilePath string
}

func inferTaskHint(run model.RunState, userMessage string) taskHint {
	if run.Brief.TaskKind == model.TaskKindDocument && strings.TrimSpace(run.Brief.TargetFilePath) != "" {
		return taskHint{
			Kind:           model.TaskKindDocument,
			TargetFilePath: run.Brief.TargetFilePath,
		}
	}

	match := markdownPathPattern.FindString(strings.TrimSpace(userMessage))
	if match == "" {
		return taskHint{Kind: model.TaskKindPlan}
	}
	if !filepath.IsAbs(match) {
		match = filepath.Join(run.CWD, match)
	}
	return taskHint{
		Kind:           model.TaskKindDocument,
		TargetFilePath: filepath.Clean(match),
	}
}

func mustCompactJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}
