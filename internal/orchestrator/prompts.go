package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"

	"panelofexperts/internal/model"
)

const briefSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["project_title", "intent_summary", "goals", "constraints", "ready_to_start", "open_questions", "manager_notes"],
  "properties": {
    "project_title": {"type": "string"},
    "intent_summary": {"type": "string"},
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
	return strings.TrimSpace(fmt.Sprintf(`
Return only JSON for a planning brief.

Stay in planning mode. Do not inspect files or edit anything during this step.
Use the latest user request and any existing brief context to set:
- project_title
- intent_summary
- goals
- constraints
- ready_to_start
- open_questions
- manager_notes

Keep the title stable once it is clear. Set ready_to_start to true only when the brief is actionable enough for expert review.

Target repository path for the later discussion: %s
Existing brief: %s
Previous manager turns: %s
Latest user request: %s
`, run.CWD, mustCompactJSON(run.Brief), mustCompactJSON(run.ManagerTurns), strings.TrimSpace(userMessage)))
}

func buildInitialProposalPrompt(run model.RunState) string {
	return strings.TrimSpace(fmt.Sprintf(`
Return only JSON for an initial planning proposal.

Stay in planning mode. You may inspect the repository at %s for grounding, but do not edit files.
Produce a concrete proposal that is specific enough for expert review.

Brief: %s
Expert panel: %s
`, run.CWD, mustCompactJSON(run.Brief), mustCompactJSON(run.Experts)))
}

func buildExpertReviewPrompt(run model.RunState, proposal model.Proposal, expert model.AgentConfig) string {
	lens := strings.ReplaceAll(string(expert.Lens), "_", " ")
	return strings.TrimSpace(fmt.Sprintf(`
Return only JSON for an expert review.

Stay in planning mode. You may inspect the repository at %s for grounding, but do not edit files.
Your review lens: %s
Review the current proposal critically but constructively. Focus on your lens and flag obvious high-risk issues.

Brief: %s
Current proposal: %s
`, run.CWD, lens, mustCompactJSON(run.Brief), mustCompactJSON(proposal)))
}

func buildMergePrompt(run model.RunState, current model.Proposal, review model.ExpertReview, expert model.AgentConfig) string {
	return strings.TrimSpace(fmt.Sprintf(`
Return only JSON for an updated planning proposal.

Stay in planning mode. You may inspect the repository at %s for grounding, but do not edit files.
Consider exactly one expert review at a time. Incorporate useful feedback, reject weak feedback, and keep the proposal coherent.
If the review does not justify a change, you may return the proposal unchanged. Set converged to true only when the proposal is materially complete and stable.

Brief: %s
Current proposal: %s
Expert reviewer: %s
Expert review: %s
`, run.CWD, mustCompactJSON(run.Brief), mustCompactJSON(current), mustCompactJSON(map[string]any{
		"name": expert.Name,
		"lens": expert.Lens,
	}), mustCompactJSON(review)))
}

func mustJSON(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

func mustCompactJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}
