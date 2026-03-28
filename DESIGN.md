# Panelofexperts TUI Design System

This document defines the target-state design system for the Panelofexperts terminal UI. It is the canonical reference for layout, semantic styling, interaction behavior, accessibility rules, and future UI extension.

## Design Goals

- Define the purpose, scope, and authority of the `Panelofexperts` TUI design system.
- Document the current UI contract and the target-state rules separately so contributors can distinguish existing constraints from intended standardization.
- Specify terminal-native design tokens and capability fallbacks, including semantic colors, text emphasis, spacing, borders, density, and non-color status cues.
- Define layout, workflow, and screen-composition rules around the app's core flow rather than a generic TUI platform.
- Describe a minimal reusable primitive set and the key workflow-specific patterns the app depends on, including async progress, status reporting, timelines, and markdown content display.
- Define keyboard interaction, focus handling, selection, feedback, and error behavior for the actual workflow surfaces in the app.
- Provide accessibility and usability guidance specific to terminal environments and degraded capability tiers.
- Include implementation mapping guidance so the design system can be adopted incrementally from the current code structure.

## Constraints

- Target deliverable is Markdown at `/Users/adamharris/dev/go/panelofexperts/DESIGN.md`.
- Focus on the repository's TUI app design system, not broader product strategy.
- Use `Panelofexperts` as the stable project title.
- Proposal must remain actionable even before repository verification, while explicitly flagging reviewer-supplied assumptions to confirm during execution.
- Avoid over-specifying speculative components that are not clearly justified by the current app workflow.

## 1. Reframe the document as a governing spec with three layers

Structure `DESIGN.md` into `Current UI contract`, `Target-state rules`, and `Implementation mapping`. This resolves the current-vs-target ambiguity and lets the document govern immediate work without pretending the full system already exists in code.

## 2. Anchor scope to the app's core workflow

Center the document on the reviewer-reported workflow surfaces: setup, manager brief, discussion monitoring, and final results. Define the design system around this shell and these flows first, instead of treating the app like a general-purpose TUI framework.

## 3. Define terminal constraints and a capability matrix

Add an explicit support matrix covering color capability tiers, Unicode versus ASCII border fallbacks, narrow-width behavior, and non-color semantic cues. Make fallback behavior normative so contributors know how design decisions degrade across terminal environments.

## 4. Specify a minimal primitive set for the normative core

Reduce the core component catalog to primitives that are likely to matter immediately: app shell, header, bordered panel, viewport/content region, form field, text input, status row, timeline entry, banner/message, loading state, empty/error states, and markdown content surface. Move speculative components into a future appendix or `only if introduced` section.

## 5. Document workflow-specific states and interaction patterns

Expand beyond generic navigation to cover async orchestration, waiting summaries, progress updates, status transitions, recoverable errors, retry affordances, and result presentation. Define keyboard behavior, focus restoration, and feedback timing in the context of these workflow states.

## 6. Define tokens with implementation-minded semantics

Keep token guidance semantic rather than palette-first, but tie it to realistic TUI usage: background, panel surface, border, body text, muted text, focus, selection, success, warning, error, info, disabled, and emphasis rules. Include spacing, border, and density conventions that work in compact terminal layouts and explicitly describe monochrome or low-color fallbacks.

## 7. Add code-level adoption guidance

Include a dedicated implementation-mapping section describing where shared styles, tokens, and reusable primitives should land once introduced, how inline styles should be consolidated, and when logic should remain in the current screen/model structure versus when a primitive should be extracted. This should give contributors an incremental adoption path rather than a big-bang rewrite mandate.

## 8. Include concrete examples tied to the actual shell

Use a small number of Markdown-native diagrams and examples for app shell anatomy, panel composition, timeline/status rendering, and keyboard conventions. Examples should clarify the core workflow surfaces rather than showcase a broad library of hypothetical widgets.

## 9. Finish with review and change-control criteria

End the document with a checklist for new screens and feature work: capability fallback coverage, focus visibility, non-color status cues, consistent primitives, and whether a new UI need belongs in the normative core or only in an appendix. Also define how deviations from the design system should be documented.

## Risks

- The architecture review introduces repo-specific assumptions that still need verification during execution; if those assumptions are wrong, some scope decisions may need adjustment.
- If `DESIGN.md` is still written too aspirationally, it will not govern near-term implementation in a codebase that currently lacks a theme or component layer.
- If the document includes too many speculative components, maintenance cost will rise and contributors may ignore the normative core.
- If the capability matrix is vague, terminal fallback behavior will remain inconsistent across environments.
- If implementation mapping is underspecified, the design system may read clearly but fail to influence actual code structure.

## Consensus Notes

- Accepted: narrow the proposal around the app's reported workflow rather than a generic TUI platform.
- Accepted: split the document into current contract, target-state rules, and implementation mapping to avoid immediate drift.
- Accepted: reduce the normative component core to a minimal primitive set and push speculative widgets out of the core.
- Accepted: add a terminal capability matrix and make degraded behavior explicit.
- Accepted: strengthen implementation mapping so contributors know how the design system lands in code.
- Rejected as too strong in its current form: treating the review's repository description as fully verified fact. The revised plan uses it as a working assumption to confirm during execution, not as already-inspected truth.
- Risk/QA review produced no actionable concerns or recommendations, so no proposal changes were justified in this iteration.

## Open Questions

- Should the canonical document be explicitly authoritative for current behavior first, with target-state guidance as a secondary layer, or should both layers carry equal weight?
- What terminal capability baseline should be treated as the primary supported environment: full ANSI color with Unicode, reduced color, or a broader fallback-first stance?
- How much of the reviewer-reported current workflow architecture should be treated as stable enough to encode directly before execution-time verification?
- Should Markdown examples include representative screen mockups for setup, monitoring, and results, or stay limited to anatomy and state snippets?
