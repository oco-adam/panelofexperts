package ui

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"panelofexperts/internal/model"
	"panelofexperts/internal/orchestrator"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

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

type briefStubProvider struct {
	id model.ProviderID
}

func (s briefStubProvider) ID() model.ProviderID { return s.id }

func (s briefStubProvider) Detect(context.Context) model.Capability {
	return model.Capability{
		Provider:      s.id,
		DisplayName:   model.ProviderDisplayName(s.id),
		Available:     true,
		Authenticated: true,
		Summary:       "ready",
	}
}

func (s briefStubProvider) Run(_ context.Context, request model.Request, progress chan<- model.ProgressEvent) (model.Result, error) {
	if request.OutputKind == "brief" {
		return model.Result{
			Stdout: `{"project_title":"Panel UI","intent_summary":"Build the first brief.","task_kind":"plan","target_file_path":"","goals":["Ship the app"],"constraints":[],"ready_to_start":true,"open_questions":[],"manager_notes":"Ready."}`,
		}, nil
	}
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
	if m.setup.Manager != model.ProviderCodex {
		t.Fatalf("expected Codex to be the preferred default manager, got %q", m.setup.Manager)
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
	monitorView = stripANSI(monitorView)
	if !strings.Contains(monitorView, "Panel UI") || !strings.Contains(monitorView, "Waiting on Claude expert and Gemini expert reviews") {
		t.Fatalf("expected monitor view to surface title and waiting summary, got:\n%s", monitorView)
	}

	run.FinalProposal = &model.Proposal{Title: "Final proposal"}
	run.FinalMarkdown = "# Final proposal\n"
	run.FinalMarkdownPath = "/tmp/final.md"
	run.DeliverablePath = "/tmp/DESIGN.md"
	run.Status = model.RunStatusComplete
	updated, _ = m.Update(discussionDoneMsg{Run: run})
	m = updated.(Model)
	if m.screen != screenResults {
		t.Fatalf("expected results screen, got %v", m.screen)
	}
	resultsView := stripANSI(m.View().Content)
	if !strings.Contains(resultsView, "# Final proposal") || !strings.Contains(resultsView, "/tmp/DESIGN.md") {
		t.Fatalf("expected final markdown to render in results view, got:\n%s", resultsView)
	}
}

func TestTypingLetterSDoesNotStartDiscussion(t *testing.T) {
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
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.screen != screenBrief {
		t.Fatalf("expected brief screen, got %v", m.screen)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)
	if m.screen != screenBrief {
		t.Fatalf("expected to remain on brief screen when typing s, got %v", m.screen)
	}
	if m.inFlight {
		t.Fatal("expected typing s to not start the discussion")
	}
}

func TestSetupScreenShowsInitialIntentInput(t *testing.T) {
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

	view := stripANSI(m.View().Content)
	for _, expected := range []string{
		"Initial Intent",
		"Tell the manager what you want to accomplish",
		"Create the run and this message will be sent to the manager immediately.",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected setup view to contain %q, got:\n%s", expected, view)
		}
	}
}

func TestBriefQuestionFlowUsesStructuredAnswerPrompt(t *testing.T) {
	tempDir := t.TempDir()
	engine := orchestrator.NewEngine(
		stubProvider{id: model.ProviderCodex},
		stubProvider{id: model.ProviderClaude},
		stubProvider{id: model.ProviderGemini},
	)
	m := New(engine, tempDir, OutputRoot(tempDir))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m.screen = screenBrief
	m.run = model.NewRunState(
		"run-123",
		tempDir,
		OutputRoot(tempDir),
		10,
		model.AgentConfig{ID: "manager", Name: "Manager (Codex CLI)", Role: model.RoleManager, Provider: model.ProviderCodex},
		[]model.AgentConfig{},
	)
	m.run.Brief.OpenQuestions = []string{
		"Should DESIGN.md describe the current UI or the target-state UI?",
		"Who is the intended audience?",
	}
	m.syncBriefInput()
	m.refreshRunViews()

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Manager Question 1 of 2") {
		t.Fatalf("expected brief view to show the current manager question, got:\n%s", view)
	}
	if m.input.Placeholder != "Answer the current manager question" {
		t.Fatalf("expected question-specific placeholder, got %q", m.input.Placeholder)
	}
	if m.input.Prompt != "A> " {
		t.Fatalf("expected answer prompt, got %q", m.input.Prompt)
	}

	submission := m.briefSubmissionText("Target-state first, with implementation drift called out explicitly.")
	for _, expected := range []string{
		"The user answered one manager follow-up question",
		"Question: Should DESIGN.md describe the current UI or the target-state UI?",
		"Answer: Target-state first, with implementation drift called out explicitly.",
		"Update the brief.",
	} {
		if !strings.Contains(submission, expected) {
			t.Fatalf("expected structured submission to contain %q, got:\n%s", expected, submission)
		}
	}
}

func TestBriefViewFitsShortWindowAndKeepsReplyVisible(t *testing.T) {
	tempDir := t.TempDir()
	engine := orchestrator.NewEngine(
		stubProvider{id: model.ProviderCodex},
		stubProvider{id: model.ProviderClaude},
		stubProvider{id: model.ProviderGemini},
	)
	m := New(engine, tempDir, OutputRoot(tempDir))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	m = updated.(Model)

	m.screen = screenBrief
	m.run = model.NewRunState(
		"run-brief",
		tempDir,
		OutputRoot(tempDir),
		5,
		model.AgentConfig{ID: "manager", Name: "Manager (Codex CLI)", Role: model.RoleManager, Provider: model.ProviderCodex},
		[]model.AgentConfig{},
	)
	m.run.Status = model.RunStatusWaiting
	m.run.WaitingSummary = "Waiting for the next user action"
	m.run.Brief.Goals = []string{
		"Goal one", "Goal two", "Goal three", "Goal four", "Goal five", "Goal six",
	}
	m.run.Brief.Constraints = []string{
		"Constraint one", "Constraint two", "Constraint three", "Constraint four",
	}
	m.run.Brief.OpenQuestions = []string{
		"Which platforms are in scope for launch?",
		"What update experience is required on desktop and mobile?",
	}
	m.run.ManagerTurns = []model.ManagerTurn{
		{Timestamp: time.Now().UTC(), UserMessage: "Initial request"},
	}
	m.syncBriefInput()
	m.refreshRunViews()

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Manager Question 1 of 2") {
		t.Fatalf("expected question panel to remain visible, got:\n%s", view)
	}
	if !strings.Contains(view, "Reply") || !strings.Contains(view, "A> ") {
		t.Fatalf("expected reply panel to remain visible, got:\n%s", view)
	}
	if height := lipgloss.Height(view); height > m.height {
		t.Fatalf("expected brief view height <= window height (%d), got %d\n%s", m.height, height, view)
	}
}

func TestMonitorViewAutoScrollsTimelineAndFitsWindow(t *testing.T) {
	tempDir := t.TempDir()
	engine := orchestrator.NewEngine(
		stubProvider{id: model.ProviderCodex},
		stubProvider{id: model.ProviderClaude},
		stubProvider{id: model.ProviderGemini},
	)
	m := New(engine, tempDir, OutputRoot(tempDir))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	m = updated.(Model)

	run := model.NewRunState(
		"run-monitor",
		tempDir,
		OutputRoot(tempDir),
		5,
		model.AgentConfig{ID: "manager", Name: "Manager (Codex CLI)", Role: model.RoleManager, Provider: model.ProviderCodex},
		[]model.AgentConfig{
			{ID: "expert-1", Name: "Expert 1 (Codex CLI)", Role: model.RoleExpert, Provider: model.ProviderCodex},
			{ID: "expert-2", Name: "Expert 2 (Claude CLI)", Role: model.RoleExpert, Provider: model.ProviderClaude},
			{ID: "expert-3", Name: "Expert 3 (Gemini CLI)", Role: model.RoleExpert, Provider: model.ProviderGemini},
		},
	)
	run.ProjectTitle = "Panel UI"
	run.Status = model.RunStatusRunning
	run.CurrentPhase = "expert_reviews"
	run.CurrentRound = 1
	run.WaitingSummary = "Waiting on Expert 2 (Claude CLI)"
	for i := 1; i <= 24; i++ {
		run.Timeline = append(run.Timeline, model.TimelineEntry{
			Timestamp: time.Now().UTC().Add(time.Duration(i) * time.Second),
			Text:      fmt.Sprintf("timeline event %02d", i),
		})
	}

	m.screen = screenMonitor
	m.run = run
	m.inFlight = true
	m.refreshRunViews()

	if !strings.Contains(stripANSI(m.timelineView.View()), "timeline event 24") {
		t.Fatalf("expected timeline viewport to show the latest event, got:\n%s", stripANSI(m.timelineView.View()))
	}

	m.run.Timeline = append(m.run.Timeline, model.TimelineEntry{
		Timestamp: time.Now().UTC().Add(25 * time.Second),
		Text:      "timeline event 25",
	})
	m.refreshRunViews()

	timelineView := stripANSI(m.timelineView.View())
	if !strings.Contains(timelineView, "timeline event 25") {
		t.Fatalf("expected timeline viewport to auto-scroll to the newest event, got:\n%s", timelineView)
	}

	view := stripANSI(m.View().Content)
	if height := lipgloss.Height(view); height > m.height {
		t.Fatalf("expected monitor view height <= window height (%d), got %d\n%s", m.height, height, view)
	}
}

func TestMonitorViewShowsFailureReasonNearTop(t *testing.T) {
	tempDir := t.TempDir()
	engine := orchestrator.NewEngine(
		stubProvider{id: model.ProviderCodex},
		stubProvider{id: model.ProviderClaude},
		stubProvider{id: model.ProviderGemini},
	)
	m := New(engine, tempDir, OutputRoot(tempDir))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	m = updated.(Model)

	run := model.NewRunState(
		"run-failed",
		tempDir,
		OutputRoot(tempDir),
		5,
		model.AgentConfig{ID: "manager", Name: "Manager (Codex CLI)", Role: model.RoleManager, Provider: model.ProviderCodex},
		[]model.AgentConfig{},
	)
	run.ProjectTitle = "Panel UI"
	run.Status = model.RunStatusFailed
	run.CurrentPhase = "manager_merge_failed"
	run.WaitingSummary = "codex timed out after 5m0s"
	run.FailureSummary = "codex timed out after 5m0s"
	run.AgentStatuses[run.Manager.ID] = model.AgentStatus{
		AgentID:  run.Manager.ID,
		Name:     run.Manager.Name,
		Role:     run.Manager.Role,
		Provider: run.Manager.Provider,
		State:    model.AgentStateError,
		LastStep: "merge_failed",
		Summary:  "codex timed out after 5m0s",
	}
	m.screen = screenMonitor
	m.run = run
	m.refreshRunViews()

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Waiting: codex timed out after 5m0s") {
		t.Fatalf("expected waiting line to show failure reason, got:\n%s", view)
	}
	if !strings.Contains(view, "Failure: codex timed out after 5m0s") {
		t.Fatalf("expected failure banner near the top, got:\n%s", view)
	}
}

func stripANSI(input string) string {
	return ansiPattern.ReplaceAllString(input, "")
}

func TestSetupIntentStartsFirstBriefUpdate(t *testing.T) {
	tempDir := t.TempDir()
	engine := orchestrator.NewEngine(
		briefStubProvider{id: model.ProviderCodex},
		briefStubProvider{id: model.ProviderClaude},
		briefStubProvider{id: model.ProviderGemini},
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

	m.setup.Focus = 6
	m.syncSetupInputFocus()
	m.setupInput.SetValue("Create the first brief")

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.screen != screenBrief {
		t.Fatalf("expected to advance to the brief screen, got %v", m.screen)
	}
	if !m.inFlight {
		t.Fatal("expected initial setup intent to start the first brief update immediately")
	}

	timeout := time.After(2 * time.Second)
	for {
		select {
		case event := <-m.events:
			switch msg := event.(type) {
			case briefDoneMsg:
				if msg.Err != nil {
					t.Fatalf("unexpected brief error: %v", msg.Err)
				}
				if msg.Run.ProjectTitle != "Panel UI" {
					t.Fatalf("expected manager brief update to complete, got project title %q", msg.Run.ProjectTitle)
				}
				return
			}
		case <-timeout:
			t.Fatal("timed out waiting for the automatic brief update")
		}
	}
}
