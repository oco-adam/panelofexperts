package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"panelofexperts/internal/model"
	"panelofexperts/internal/orchestrator"
	"panelofexperts/internal/render"
)

type Screen int

const (
	screenLoading Screen = iota
	screenSetup
	screenBrief
	screenMonitor
	screenResults
)

type capabilitiesMsg struct {
	Capabilities map[model.ProviderID]model.Capability
}

type snapshotMsg struct {
	Run model.RunState
}

type briefDoneMsg struct {
	Run model.RunState
	Err error
}

type discussionDoneMsg struct {
	Run model.RunState
	Err error
}

type SetupState struct {
	Focus       int
	Manager     model.ProviderID
	Experts     []model.ProviderID
	ExpertCount int
	MaxRounds   int
	CWD         string
	OutputRoot  string
}

type Model struct {
	engine       *orchestrator.Engine
	screen       Screen
	width        int
	height       int
	events       chan tea.Msg
	err          string
	inFlight     bool
	capabilities map[model.ProviderID]model.Capability
	setup        SetupState
	run          model.RunState

	input          textinput.Model
	briefViewport  viewport.Model
	statusViewport viewport.Model
	timelineView   viewport.Model
	resultViewport viewport.Model
	spin           spinner.Model

	headerStyle  lipgloss.Style
	panelStyle   lipgloss.Style
	focusStyle   lipgloss.Style
	mutedStyle   lipgloss.Style
	errorStyle   lipgloss.Style
	successStyle lipgloss.Style
}

func New(engine *orchestrator.Engine, cwd, outputRoot string) Model {
	input := textinput.New()
	input.Focus()
	input.Prompt = "> "
	input.CharLimit = 0
	input.SetWidth(80)

	spin := spinner.New()

	return Model{
		engine: engine,
		screen: screenLoading,
		events: make(chan tea.Msg, 128),
		setup: SetupState{
			ExpertCount: 3,
			MaxRounds:   10,
			CWD:         cwd,
			OutputRoot:  outputRoot,
			Experts:     []model.ProviderID{model.ProviderClaude, model.ProviderGemini, model.ProviderCodex},
		},
		input: input,
		spin:  spin,
		headerStyle: lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1),
		panelStyle: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			Padding(0, 1),
		focusStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("10")),
		mutedStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")),
		errorStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Bold(true),
		successStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true),
	}
}

func (m Model) Init() tea.Cmd {
	go func() {
		m.events <- capabilitiesMsg{Capabilities: m.engine.DetectCapabilities(context.Background())}
	}()
	return tea.Batch(waitForEvent(m.events), m.spin.Tick)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
	case spinner.TickMsg:
		if m.inFlight {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			cmds = append(cmds, cmd)
		}
	case capabilitiesMsg:
		m.capabilities = msg.Capabilities
		m.screen = screenSetup
		m.initSetupDefaults()
		m.syncBriefInput()
	case snapshotMsg:
		m.run = msg.Run
		m.syncBriefInput()
		m.refreshRunViews()
		if m.run.Status == model.RunStatusRunning && m.screen != screenResults {
			m.screen = screenMonitor
		}
	case briefDoneMsg:
		m.inFlight = false
		if msg.Err != nil {
			m.err = msg.Err.Error()
		} else {
			m.err = ""
			m.run = msg.Run
			m.syncBriefInput()
			m.refreshRunViews()
			m.screen = screenBrief
		}
	case discussionDoneMsg:
		m.inFlight = false
		if msg.Err != nil {
			m.err = msg.Err.Error()
			m.screen = screenMonitor
		} else {
			m.err = ""
			m.run = msg.Run
			m.refreshRunViews()
			m.screen = screenResults
		}
	case tea.KeyMsg:
		switch m.screen {
		case screenSetup:
			cmds = append(cmds, m.updateSetup(msg))
		case screenBrief:
			cmds = append(cmds, m.updateBrief(msg))
		case screenMonitor:
			cmds = append(cmds, m.updateMonitor(msg))
		case screenResults:
			cmds = append(cmds, m.updateResults(msg))
		}
	}

	if m.screen == screenBrief {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}
	if m.screen == screenMonitor {
		var cmd tea.Cmd
		m.timelineView, cmd = m.timelineView.Update(msg)
		cmds = append(cmds, cmd)
		m.statusViewport, cmd = m.statusViewport.Update(msg)
		cmds = append(cmds, cmd)
	}
	if m.screen == screenResults {
		var cmd tea.Cmd
		m.resultViewport, cmd = m.resultViewport.Update(msg)
		cmds = append(cmds, cmd)
	}
	if m.screen == screenBrief {
		var cmd tea.Cmd
		m.briefViewport, cmd = m.briefViewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	cmds = append(cmds, waitForEvent(m.events))
	return m, tea.Batch(cmds...)
}

func (m Model) View() tea.View {
	var content string
	if m.width == 0 || m.height == 0 {
		content = "Loading..."
		return tea.NewView(content)
	}

	switch m.screen {
	case screenLoading:
		content = m.headerStyle.Render("Detecting providers...")
	case screenSetup:
		content = m.viewSetup()
	case screenBrief:
		content = m.viewBrief()
	case screenMonitor:
		content = m.viewMonitor()
	case screenResults:
		content = m.viewResults()
	}
	return tea.NewView(content)
}

func waitForEvent(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-events
	}
}

func (m *Model) resize() {
	contentHeight := max(6, m.height-8)
	panelWidth := max(30, m.width-4)
	m.briefViewport = viewport.New(viewport.WithWidth(panelWidth), viewport.WithHeight(contentHeight-4))
	m.statusViewport = viewport.New(viewport.WithWidth(max(30, panelWidth/2-1)), viewport.WithHeight(contentHeight))
	m.timelineView = viewport.New(viewport.WithWidth(max(30, panelWidth/2-1)), viewport.WithHeight(contentHeight))
	m.resultViewport = viewport.New(viewport.WithWidth(panelWidth), viewport.WithHeight(contentHeight))
	m.refreshRunViews()
}

func (m *Model) initSetupDefaults() {
	available := m.availableProviders()
	if len(available) == 0 {
		available = model.AllProviders()
	}
	m.setup.Manager = preferredManager(available)
	m.setup.Experts = make([]model.ProviderID, 3)
	for i := range m.setup.Experts {
		m.setup.Experts[i] = available[i%len(available)]
	}
}

func (m Model) availableProviders() []model.ProviderID {
	providers := []model.ProviderID{}
	for _, id := range model.AllProviders() {
		capability, ok := m.capabilities[id]
		if ok && capability.Available {
			providers = append(providers, id)
		}
	}
	return providers
}

func (m *Model) updateSetup(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c", "q":
		return tea.Quit
	case "tab", "down", "j":
		m.setup.Focus = (m.setup.Focus + 1) % 6
	case "shift+tab", "up", "k":
		m.setup.Focus = (m.setup.Focus + 5) % 6
	case "left", "h":
		m.adjustSetup(-1)
	case "right", "l":
		m.adjustSetup(1)
	case "enter":
		if m.inFlight {
			return nil
		}
		run, err := m.engine.NewRun(orchestrator.NewRunOptions{
			CWD:             m.setup.CWD,
			OutputRoot:      m.setup.OutputRoot,
			MaxRounds:       m.setup.MaxRounds,
			ManagerProvider: m.setup.Manager,
			ExpertProviders: append([]model.ProviderID{}, m.setup.Experts[:m.setup.ExpertCount]...),
		})
		if err != nil {
			m.err = err.Error()
			return nil
		}
		m.err = ""
		m.run = run
		m.syncBriefInput()
		m.refreshRunViews()
		m.screen = screenBrief
	}
	return nil
}

func (m *Model) adjustSetup(delta int) {
	available := m.availableProviders()
	if len(available) == 0 {
		return
	}

	cycle := func(current model.ProviderID) model.ProviderID {
		index := 0
		for i, provider := range available {
			if provider == current {
				index = i
				break
			}
		}
		index = (index + delta + len(available)) % len(available)
		return available[index]
	}

	switch m.setup.Focus {
	case 0:
		m.setup.Manager = cycle(m.setup.Manager)
	case 1, 2, 3:
		idx := m.setup.Focus - 1
		m.setup.Experts[idx] = cycle(m.setup.Experts[idx])
	case 4:
		m.setup.ExpertCount += delta
		if m.setup.ExpertCount < 2 {
			m.setup.ExpertCount = 2
		}
		if m.setup.ExpertCount > 3 {
			m.setup.ExpertCount = 3
		}
	case 5:
		m.setup.MaxRounds += delta
		if m.setup.MaxRounds < 1 {
			m.setup.MaxRounds = 1
		}
	}
}

func (m *Model) updateBrief(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c", "q":
		return tea.Quit
	case "enter":
		if m.inFlight || strings.TrimSpace(m.input.Value()) == "" {
			return nil
		}
		message := m.briefSubmissionText(strings.TrimSpace(m.input.Value()))
		m.input.SetValue("")
		m.err = ""
		m.inFlight = true
		go func(run model.RunState, text string) {
			updated, err := m.engine.UpdateBrief(context.Background(), run, text, func(snapshot model.RunState) {
				m.events <- snapshotMsg{Run: snapshot}
			})
			m.events <- briefDoneMsg{Run: updated, Err: err}
		}(m.run, message)
		return nil
	case "ctrl+s":
		if m.inFlight {
			return nil
		}
		m.err = ""
		m.inFlight = true
		m.screen = screenMonitor
		go func(run model.RunState) {
			updated, err := m.engine.RunDiscussion(context.Background(), run, func(snapshot model.RunState) {
				m.events <- snapshotMsg{Run: snapshot}
			})
			m.events <- discussionDoneMsg{Run: updated, Err: err}
		}(m.run)
	}
	return nil
}

func (m *Model) updateMonitor(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c", "q":
		return tea.Quit
	case "r":
		if m.run.FinalProposal != nil {
			m.screen = screenResults
		}
	}
	return nil
}

func (m *Model) updateResults(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c", "q":
		return tea.Quit
	case "m":
		m.screen = screenMonitor
	}
	return nil
}

func (m *Model) refreshRunViews() {
	if m.briefViewport.Width() > 0 {
		m.briefViewport.SetContent(m.briefContent())
	}
	if m.statusViewport.Width() > 0 {
		m.statusViewport.SetContent(m.statusContent())
	}
	if m.timelineView.Width() > 0 {
		m.timelineView.SetContent(m.timelineContent())
	}
	if m.resultViewport.Width() > 0 {
		m.resultViewport.SetContent(m.resultContent())
	}
}

func (m Model) viewSetup() string {
	lines := []string{
		m.header("Panel of Experts", "Setup"),
		"",
		m.renderSetupField(0, "Manager", model.ProviderDisplayName(m.setup.Manager)),
		m.renderSetupField(1, "Expert 1", model.ProviderDisplayName(m.setup.Experts[0])),
		m.renderSetupField(2, "Expert 2", model.ProviderDisplayName(m.setup.Experts[1])),
		m.renderSetupField(3, "Expert 3", m.thirdExpertDisplay()),
		m.renderSetupField(4, "Expert count", fmt.Sprintf("%d", m.setup.ExpertCount)),
		m.renderSetupField(5, "Max rounds", fmt.Sprintf("%d", m.setup.MaxRounds)),
		"",
		fmt.Sprintf("Workspace: %s", m.setup.CWD),
		fmt.Sprintf("Output root: %s", m.setup.OutputRoot),
		"",
		"Provider status:",
	}
	for _, id := range model.AllProviders() {
		capability := m.capabilities[id]
		status := "missing"
		if capability.Available {
			status = "available"
			if capability.Authenticated {
				status = "ready"
			}
		}
		lines = append(lines, fmt.Sprintf("- %s: %s (%s)", model.ProviderDisplayName(id), capability.Summary, status))
	}
	lines = append(lines, "", m.mutedStyle.Render("Use tab/j/k to move, h/l to change values, enter to create the run."))
	if m.err != "" {
		lines = append(lines, "", m.errorStyle.Render(m.err))
	}
	return strings.Join(lines, "\n")
}

func (m Model) viewBrief() string {
	body := []string{
		m.header(m.run.ProjectTitle, "Manager Brief"),
		"",
		fmt.Sprintf("Run: %s", m.run.ID),
		fmt.Sprintf("Status: %s", m.run.Status),
		fmt.Sprintf("Waiting: %s", m.run.WaitingSummary),
		"",
		m.panelStyle.Width(m.width - 4).Render(m.briefViewport.View()),
		"",
	}
	if question, index, total := m.activeBriefQuestion(); question != "" {
		body = append(body,
			m.panelStyle.Width(m.width-4).Render(strings.Join([]string{
				fmt.Sprintf("Manager Question %d of %d", index+1, total),
				"",
				question,
			}, "\n")),
			"",
		)
	}
	if m.inFlight {
		body = append(body, fmt.Sprintf("%s Manager is updating the brief", m.spin.View()))
	} else if question, index, total := m.activeBriefQuestion(); question != "" {
		body = append(body, m.mutedStyle.Render(fmt.Sprintf("Enter submits your answer to manager question %d of %d. Press ctrl+s to start the discussion anyway.", index+1, total)))
	} else {
		body = append(body, m.mutedStyle.Render("Enter sends the next message to the manager. Press ctrl+s to start the discussion."))
	}
	body = append(body, m.input.View())
	if m.err != "" {
		body = append(body, "", m.errorStyle.Render(m.err))
	}
	return strings.Join(body, "\n")
}

func (m Model) viewMonitor() string {
	left := m.panelStyle.Width(max(30, m.width/2-3)).Render(m.statusViewport.View())
	right := m.panelStyle.Width(max(30, m.width/2-3)).Render(m.timelineView.View())
	main := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)

	lines := []string{
		m.header(m.run.ProjectTitle, "Discussion Monitor"),
		"",
		fmt.Sprintf("Run: %s", m.run.ID),
		fmt.Sprintf("Phase: %s", m.run.CurrentPhase),
		fmt.Sprintf("Iteration: %s", m.run.DisplayRound()),
		fmt.Sprintf("Status: %s", m.run.Status),
		fmt.Sprintf("Waiting: %s", m.run.WaitingSummary),
		"",
		main,
		"",
	}
	if m.run.FinalProposal != nil {
		lines = append(lines, m.successStyle.Render("Discussion finished. Press r to view the final markdown."))
	} else if m.inFlight {
		lines = append(lines, fmt.Sprintf("%s Orchestration is running", m.spin.View()))
	}
	if m.err != "" {
		lines = append(lines, "", m.errorStyle.Render(m.err))
	}
	return strings.Join(lines, "\n")
}

func (m Model) viewResults() string {
	lines := []string{
		m.header(m.run.ProjectTitle, "Final Proposal"),
		"",
		fmt.Sprintf("Run: %s", m.run.ID),
		fmt.Sprintf("Stop reason: %s", m.run.StopReason),
		fmt.Sprintf("Final markdown: %s", m.run.FinalMarkdownPath),
	}
	if strings.TrimSpace(m.run.DeliverablePath) != "" {
		lines = append(lines, fmt.Sprintf("Deliverable file: %s", m.run.DeliverablePath))
	}
	lines = append(lines,
		"",
		m.panelStyle.Width(m.width-4).Render(m.resultViewport.View()),
		"",
		m.mutedStyle.Render("Press m to return to the monitor, q to quit."),
	)
	if m.err != "" {
		lines = append(lines, "", m.errorStyle.Render(m.err))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderSetupField(index int, label, value string) string {
	line := fmt.Sprintf("%-12s %s", label+":", value)
	if m.setup.Focus == index {
		return m.focusStyle.Render("> " + line)
	}
	return "  " + line
}

func (m Model) thirdExpertDisplay() string {
	if m.setup.ExpertCount < 3 {
		return "(disabled)"
	}
	return model.ProviderDisplayName(m.setup.Experts[2])
}

func (m Model) briefContent() string {
	if m.run.ID == "" {
		return ""
	}
	parts := []string{render.RenderBriefMarkdown(m.run.Brief)}
	if len(m.run.ManagerTurns) > 0 {
		parts = append(parts, "## Manager Turns\n")
		for _, turn := range m.run.ManagerTurns {
			parts = append(parts, fmt.Sprintf("- %s: %s", turn.Timestamp.Format(timeFormat), turn.UserMessage))
		}
	}
	return strings.Join(parts, "\n")
}

func (m *Model) syncBriefInput() {
	if question, _, _ := m.activeBriefQuestion(); question != "" {
		m.input.Placeholder = "Answer the current manager question"
		m.input.Prompt = "A> "
		return
	}
	m.input.Placeholder = "Tell the manager what you want to accomplish"
	m.input.Prompt = "> "
}

func (m Model) activeBriefQuestion() (string, int, int) {
	for i, question := range m.run.Brief.OpenQuestions {
		question = strings.TrimSpace(question)
		if question != "" {
			return question, i, len(m.run.Brief.OpenQuestions)
		}
	}
	return "", 0, 0
}

func (m Model) briefSubmissionText(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	question, _, _ := m.activeBriefQuestion()
	if question == "" {
		return input
	}
	return strings.TrimSpace(fmt.Sprintf(`
The user answered one manager follow-up question for the planning brief.

Question: %s
Answer: %s

Update the brief. Remove resolved open questions, keep unresolved questions, and adjust ready_to_start if appropriate.
`, question, input))
}

func (m Model) statusContent() string {
	if m.run.ID == "" {
		return ""
	}
	lines := []string{
		"Agent Status",
		"",
	}
	for _, agent := range m.run.AllAgents() {
		status := m.run.AgentStatuses[agent.ID]
		lines = append(lines,
			fmt.Sprintf("%s", agent.Name),
			fmt.Sprintf("  Provider: %s", model.ProviderDisplayName(agent.Provider)),
			fmt.Sprintf("  State: %s", status.State),
			fmt.Sprintf("  Step: %s", status.LastStep),
			fmt.Sprintf("  Summary: %s", emptyFallback(status.Summary, "No updates yet")),
			"",
		)
	}
	return strings.Join(lines, "\n")
}

func (m Model) timelineContent() string {
	if m.run.ID == "" {
		return ""
	}
	lines := []string{"Timeline", ""}
	for _, entry := range m.run.Timeline {
		lines = append(lines, fmt.Sprintf("[%s] %s", entry.Timestamp.Format(timeFormat), entry.Text))
	}
	return strings.Join(lines, "\n")
}

func (m Model) resultContent() string {
	if m.run.FinalMarkdown != "" {
		return m.run.FinalMarkdown
	}
	if proposal := m.run.LatestProposal(); proposal != nil {
		return render.RenderProposalMarkdown(*proposal, m.run)
	}
	return "No final proposal yet."
}

func (m Model) header(title, subtitle string) string {
	status := subtitle
	if m.inFlight {
		status += "  " + m.spin.View()
	}
	return m.headerStyle.Render(fmt.Sprintf("%s | %s", title, status))
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

const timeFormat = "15:04:05"

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func preferredManager(available []model.ProviderID) model.ProviderID {
	for _, candidate := range []model.ProviderID{model.ProviderCodex, model.ProviderClaude, model.ProviderGemini} {
		for _, provider := range available {
			if provider == candidate {
				return provider
			}
		}
	}
	if len(available) == 0 {
		return model.ProviderCodex
	}
	return available[0]
}

func OutputRoot(cwd string) string {
	return filepath.Join(cwd, ".panel-of-experts", "runs")
}
