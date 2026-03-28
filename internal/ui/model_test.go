package ui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"panelofexperts/internal/model"
	"panelofexperts/internal/orchestrator"
)

type stubProvider struct {
	id model.ProviderID
}

func (s stubProvider) ID() model.ProviderID { return s.id }

func (s stubProvider) Detect(context.Context) model.Capability {
	return model.Capability{
		Provider:      s.id,
		DisplayName:   model.ProviderDisplayName(s.id),
		Available:     true,
		Authenticated: true,
		Summary:       "ready",
	}
}

func (s stubProvider) Run(context.Context, model.Request, chan<- model.ProgressEvent) (model.Result, error) {
	return model.Result{}, nil
}

func TestModelScreenTransitionsAndStatusContent(t *testing.T) {
	tempDir := t.TempDir()
	engine := orchestrator.NewEngine(
		stubProvider{id: model.ProviderCodex},
		stubProvider{id: model.ProviderClaude},
		stubProvider{id: model.ProviderGemini},
	)
	m := New(engine, tempDir, OutputRoot(tempDir))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(capabilitiesMsg{
		Capabilities: map[model.ProviderID]model.Capability{
			model.ProviderCodex:  {Provider: model.ProviderCodex, Available: true, Authenticated: true, Summary: "ready"},
			model.ProviderClaude: {Provider: model.ProviderClaude, Available: true, Authenticated: true, Summary: "ready"},
			model.ProviderGemini: {Provider: model.ProviderGemini, Available: true, Authenticated: true, Summary: "ready"},
		},
	})
	m = updated.(Model)
	if m.screen != screenSetup {
		t.Fatalf("expected setup screen, got %v", m.screen)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.screen != screenBrief {
		t.Fatalf("expected brief screen after creating run, got %v", m.screen)
	}

	run := m.run
	run.ProjectTitle = "Panel UI"
	run.Status = model.RunStatusRunning
	run.CurrentPhase = "expert_reviews"
	run.CurrentRound = 1
	run.WaitingSummary = "Waiting on Claude expert and Gemini expert reviews"
	run.AgentStatuses[run.Manager.ID] = model.AgentStatus{
		AgentID:  run.Manager.ID,
		Name:     run.Manager.Name,
		Role:     run.Manager.Role,
		Provider: run.Manager.Provider,
		State:    model.AgentStateWaitingOnExperts,
		LastStep: "expert_reviews",
		Summary:  "Waiting on expert reviews",
	}
	updated, _ = m.Update(snapshotMsg{Run: run})
	m = updated.(Model)
	if m.screen != screenMonitor {
		t.Fatalf("expected monitor screen during running discussion, got %v", m.screen)
	}

	monitorView := m.View().Content
	if !strings.Contains(monitorView, "Panel UI") || !strings.Contains(monitorView, "Waiting on Claude expert and Gemini expert reviews") {
		t.Fatalf("expected monitor view to surface title and waiting summary, got:\n%s", monitorView)
	}

	run.FinalProposal = &model.Proposal{Title: "Final proposal"}
	run.FinalMarkdown = "# Final proposal\n"
	run.FinalMarkdownPath = "/tmp/final.md"
	run.Status = model.RunStatusComplete
	updated, _ = m.Update(discussionDoneMsg{Run: run})
	m = updated.(Model)
	if m.screen != screenResults {
		t.Fatalf("expected results screen, got %v", m.screen)
	}
	if !strings.Contains(m.View().Content, "# Final proposal") {
		t.Fatalf("expected final markdown to render in results view, got:\n%s", m.View().Content)
	}
}
