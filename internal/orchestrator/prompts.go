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
You are the manager agent for a planning-focused panel of experts. You may inspect the workspace, but do not edit files.

Your job is to refine the discussion brief after the user's latest message.

Rules:
- Stay in planning mode.
- Read the codebase when useful.
- Do not change any files.
- Return JSON only that matches the provided schema.
- Keep the project title concrete and stable once it is clear.
- Set ready_to_start to true only when the brief is actionable enough for expert review.

Current brief JSON:
%s

Previous manager turns:
%s

Latest user message:
%s
`, mustJSON(run.Brief), mustJSON(run.ManagerTurns), strings.TrimSpace(userMessage)))
}

func buildInitialProposalPrompt(run model.RunState) string {
	experts := make([]string, 0, len(run.Experts))
	for _, expert := range run.Experts {
		experts = append(experts, fmt.Sprintf("%s (%s)", expert.Name, expert.Lens))
	}
	return strings.TrimSpace(fmt.Sprintf(`
You are the manager agent for a panel-of-experts planning workflow.

Create the initial proposal for the expert panel to review.

Rules:
- Stay in planning mode and do not edit files.
- Use the workspace for context when helpful.
- Produce a plan that is specific enough for experts to critique.
- Return JSON only that matches the provided schema.

Brief JSON:
%s

Experts:
%s
`, mustJSON(run.Brief), strings.Join(experts, ", ")))
}

func buildExpertReviewPrompt(run model.RunState, proposal model.Proposal, expert model.AgentConfig) string {
	lens := strings.ReplaceAll(string(expert.Lens), "_", " ")
	return strings.TrimSpace(fmt.Sprintf(`
You are an expert reviewer in a panel-of-experts planning workflow.

Your lens: %s

Rules:
- Stay in planning mode and do not edit files.
- Review the current proposal critically but constructively.
- Focus on your lens while still flagging any obvious high-risk issues.
- Return JSON only that matches the provided schema.

Brief JSON:
%s

Current proposal JSON:
%s
`, lens, mustJSON(run.Brief), mustJSON(proposal)))
}

func buildMergePrompt(run model.RunState, current model.Proposal, review model.ExpertReview, expert model.AgentConfig) string {
	return strings.TrimSpace(fmt.Sprintf(`
You are the manager agent for a panel-of-experts planning workflow.

Update the proposal by considering exactly one expert review at a time.

Rules:
- Stay in planning mode and do not edit files.
- Incorporate useful feedback, reject weak feedback, and keep the proposal coherent.
- If the review does not justify a change, you may return the proposal unchanged.
- Set converged to true only if the proposal appears materially complete and stable.
- Return JSON only that matches the provided schema.

Brief JSON:
%s

Current proposal JSON:
%s

Expert reviewer:
%s (%s)

Expert review JSON:
%s
`, mustJSON(run.Brief), mustJSON(current), expert.Name, expert.Lens, mustJSON(review)))
}

func mustJSON(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}
