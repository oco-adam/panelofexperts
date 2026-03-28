# Panelofexperts TUI Design System

This document defines the target-state design system for the Panelofexperts terminal UI. It is the canonical reference for layout, semantic styling, interaction behavior, accessibility rules, and future UI extension.

## Design Goals

- Make DESIGN.md the canonical design-system contract for the repository's existing TUI, not a generic terminal design document.
- Replace the current scaffold content with a concise reference optimized for implementation and review.
- Document the actual screen model in the repo: loading, setup, brief, monitor, and results.
- Define visual foundations from the current implementation, including layout regions, spacing in character cells, border treatment, semantic typography, color roles, and emphasis hierarchy.
- Document only the component and primitive patterns the codebase currently uses or clearly implies: header, panel, divider, labeled line, meta badge, provider badge, status/state badge, text input, viewport region, split monitor layout, spinner/loading, and feedback states.
- Capture the keyboard-first interaction model, including setup navigation, brief submission, discussion start, monitor/results switching, and when interaction is blocked during in-flight work.
- Map visible UI behavior to actual runtime states and terminology, including RunStatus, AgentState, StopReason, and key CurrentPhase values.
- Include terminal-specific accessibility and compatibility guidance covering contrast, non-color cues, resize behavior, Unicode border assumptions, and reduced cognitive load.
- Define a practical definition of done so the final DESIGN.md stops at the right scope and remains usable as a quick reference.
- Leave clear out-of-scope notes or backlog markers for component types not currently present, instead of standardizing speculative UI.

## Constraints

- Do not call any write, edit, or create tool.
- Use only read-only repository inspection for grounding.
- The later deliverable must be a Markdown document at /Users/adamharris/dev/go/panelofexperts/DESIGN.md.
- Keep the document focused on the TUI design system and implementation contract, not broader product planning.
- Scope the primary document to components and screens that exist in the repository today; future patterns should be clearly marked as out of scope or backlog.
- Separate current-state rules, target-state guidance, and illustrative examples so contributors can tell what is normative.
- Keep the final document intentionally concise, roughly quick-reference sized rather than wiki sized.
- Treat terminal capability assumptions as part of the contract because the current UI already depends on alt-screen rendering, mouse mode enablement, Lip Gloss borders, and color semantics.

## Replace the Scaffold With a Code-Anchored Contract

Rewrite DESIGN.md as a replacement for the current generic scaffold. Open with a short contract that defines the file as canonical, explains the difference between current-state rules, target-state guidance, and examples, and states that repo reality wins over generic TUI conventions.

## Document the Actual Screen Graph First

Base the document on the five screens already present in internal/ui/model.go: loading, setup, brief, monitor, and results. For each screen, capture purpose, dominant information hierarchy, primary actions, and transition triggers so the design system mirrors the current app instead of imagined future surfaces.

## Define Terminal Foundations From Existing Behavior

Write a foundations section grounded in the current stack: alt-screen is the normal presentation mode, mouse mode is enabled but should be treated as optional enhancement because keyboard handlers drive interaction, rounded borders and Unicode dividers are expected, and resize behavior relies on flexible viewport widths rather than bespoke responsive layouts.

## Translate Hardcoded Styles Into Semantic Rules

Convert the current Lip Gloss styling into semantic design language: title treatment, subtitle treatment, panel chrome, divider semantics, label/value pairs, muted helper text, focus emphasis, and success/warning/error/info roles. Prefer semantic color-role naming over freezing every numeric palette value, while still noting the current palette as implementation context.

## Narrow the Component Catalog to Real Primitives

Standardize only the primitives that exist now or are directly implied by the code: header block, bordered panel, section divider, labeled line, meta badge, provider badge, run/agent status badge, text input, scrollable viewport, split live-activity layout, and feedback states such as loading, waiting, error, empty, and complete. Explicitly mark tables, dialogs, generalized alerts, and richer form systems as out of scope unless implementation introduces them later.

## Add a State and Interaction Matrix

Create a compact matrix mapping Screen, RunStatus, AgentState, StopReason, and major CurrentPhase values such as setup, manager_brief, brief_ready, manager_initial_proposal, expert_reviews, manager_merge, writing_deliverable, and finalized to visible UI treatment and allowed user actions. This gives implementation and review a concrete reference for focus, navigation, and state presentation.

## Capture Keyboard-First Usage Per Screen

Document the real bindings and behavior patterns already encoded in the UI: setup movement and value adjustment, enter-to-create-run, enter-to-message-manager, ctrl+s to start discussion, r to switch from monitor to results when available, m to return to monitor, and q/ctrl+c to quit. Note where typing is allowed, when inputs blur or focus, and when in-flight work suppresses actions.

## Add Accessibility, Copy, and Cognitive-Load Rules

Finish with terminal-specific accessibility guidance: meaningful state changes must not rely on color alone, labels should stay terse and scan-friendly, helper copy should explain the next action, empty and waiting states should be explicit, and dense screens should preserve clear grouping through spacing and borders rather than ornamental styling.

## Set Definition of Done and Backlog Rules

Define the stopping condition for the drafting pass: every existing screen and shared primitive in internal/ui/model.go has a corresponding rule or explicit exclusion, every visible lifecycle state has a documented presentation rule, the document stays concise enough to scan quickly, and any future-only patterns are listed in a short backlog section instead of expanded into full standards.

## Risks

- If DESIGN.md drifts back into generic TUI advice, it will duplicate the current scaffold problem and fail to govern implementation.
- If the component catalog expands beyond what the repo currently renders, the document will become misleading and harder to maintain.
- If current-state rules and target-state guidance are mixed without labels, contributors will not know what is binding today.
- If semantic roles are defined without acknowledging current terminal assumptions, future changes may break borders, color fallback, or resize behavior accidentally.
- If the document becomes too long, it will function as shelf-ware instead of a reviewable implementation reference.
- Because style ownership is still centralized in internal/ui/model.go, the design system may drift unless the document explicitly calls out update triggers for new screens, states, or shared primitives.

## Consensus Notes

- Accepted the execution review's main criticism that the previous proposal was too generic and needed real repo grounding before it could be trusted.
- Accepted the recommendation to scope the document ruthlessly to components and screens that actually exist in the repository today.
- Accepted the need for a concrete definition of done so the drafting pass has a natural stopping point and does not balloon into a wiki.
- Accepted the recommendation to resolve the previously blocking questions about framework, terminal assumptions, and current-vs-aspirational stance during planning rather than after drafting begins.
- Accepted the idea of keeping the final document quick-reference sized instead of treating every possible TUI pattern as first-class.
- Rejected treating broad component categories like tables, dialogs, and generalized alert systems as current standards because the inspected UI does not justify them.
- Rejected a formal two-deliverable split for this task because the request is for a single canonical DESIGN.md; a short backlog or out-of-scope section is sufficient.
- Rejected staying framework-agnostic now that the repo has been inspected; Bubble Tea, Bubbles, and Lip Gloss should be named where they materially affect the contract.
- Risk/QA review produced no actionable concerns or blocking risks, so the proposal remains unchanged in substance.

## Open Questions

- Should the final document merely record the current centralized style ownership in internal/ui/model.go, or should it also prescribe a future extraction seam for shared design primitives?
- Should DESIGN.md include the current numeric Lip Gloss color values in an appendix, or keep the canonical guidance strictly semantic and treat the numeric palette as implementation detail?
- Should narrow-terminal fallback behavior be specified now as a target-state guideline, given the current implementation mostly scales widths rather than switching to a distinct single-column layout?
