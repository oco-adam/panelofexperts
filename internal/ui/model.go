package ui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"panelofexperts/internal/appenv"
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
	MergeMode   model.MergeStrategy
	CWD         string
	OutputRoot  string
}

type Model struct {
	engine       *orchestrator.Engine
	screen       Screen
	width        int
	height       int
	layout       screenLayout
	events       chan tea.Msg
	err          string
	inFlight     bool
	capabilities map[model.ProviderID]model.Capability
	setup        SetupState
	run          model.RunState

	setupInput     textinput.Model
	input          textinput.Model
	briefViewport  viewport.Model
	statusViewport viewport.Model
	timelineView   viewport.Model
	resultViewport viewport.Model
	spin           spinner.Model

	theme  theme
	chrome uiChrome
	motion uiMotion
}

const setupFieldCount = 8

func New(engine *orchestrator.Engine, cwd, outputRoot string) Model {
	theme := newTheme()
	chrome := newChrome(theme)
	motion := newMotion(theme)
	inputStyles := theme.inputStyles()

	input := textinput.New()
	input.Focus()
	input.Prompt = "> "
	input.CharLimit = 0
	input.SetWidth(80)
	input.SetStyles(inputStyles)

	setupInput := textinput.New()
	setupInput.Prompt = "Intent> "
	setupInput.Placeholder = "Tell the manager what you want to accomplish"
	setupInput.CharLimit = 0
	setupInput.SetWidth(80)
	setupInput.SetStyles(inputStyles)
	setupInput.Blur()

	return Model{
		engine: engine,
		screen: screenLoading,
		layout: newScreenLayout(0, 0),
		events: make(chan tea.Msg, 128),
		setup: SetupState{
			ExpertCount: 3,
			MaxRounds:   5,
			MergeMode:   model.MergeStrategyTogether,
			CWD:         cwd,
			OutputRoot:  outputRoot,
			Experts:     []model.ProviderID{model.ProviderClaude, model.ProviderGemini, model.ProviderCodex},
		},
		setupInput:     setupInput,
		input:          input,
		briefViewport:  viewport.New(),
		statusViewport: viewport.New(),
		timelineView:   viewport.New(),
		resultViewport: viewport.New(),
		spin:           motion.newSpinner(),
		theme:          theme,
		chrome:         chrome,
		motion:         motion,
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
		m.syncSetupInputFocus()
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
	if m.screen == screenSetup && m.setup.Focus == 7 {
		var cmd tea.Cmd
		m.setupInput, cmd = m.setupInput.Update(msg)
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
		view := tea.NewView(content)
		view.AltScreen = true
		view.MouseMode = tea.MouseModeCellMotion
		return view
	}

	switch m.screen {
	case screenLoading:
		content = m.viewLoading()
	case screenSetup:
		content = m.viewSetup()
	case screenBrief:
		content = m.viewBrief()
	case screenMonitor:
		content = m.viewMonitor()
	case screenResults:
		content = m.viewResults()
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func waitForEvent(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-events
	}
}

func (m *Model) resize() {
	m.layout = newScreenLayout(m.width, m.height)
	m.briefViewport.SetWidth(m.layout.contentWidth)
	m.statusViewport.SetWidth(m.layout.monitorStatusW)
	m.timelineView.SetWidth(m.layout.monitorTimelineW)
	m.resultViewport.SetWidth(m.layout.contentWidth)
	m.input.SetWidth(m.layout.inputWidth)
	m.setupInput.SetWidth(m.layout.inputWidth)
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
	if m.setup.Focus == 7 {
		switch msg.String() {
		case "ctrl+c", "q":
			return tea.Quit
		case "tab", "down":
			m.setup.Focus = (m.setup.Focus + 1) % setupFieldCount
			m.syncSetupInputFocus()
		case "shift+tab", "up":
			m.setup.Focus = (m.setup.Focus + setupFieldCount - 1) % setupFieldCount
			m.syncSetupInputFocus()
		case "enter":
			return m.createRunFromSetup()
		}
		return nil
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return tea.Quit
	case "tab", "down", "j":
		m.setup.Focus = (m.setup.Focus + 1) % setupFieldCount
		m.syncSetupInputFocus()
	case "shift+tab", "up", "k":
		m.setup.Focus = (m.setup.Focus + setupFieldCount - 1) % setupFieldCount
		m.syncSetupInputFocus()
	case "left", "h":
		m.adjustSetup(-1)
	case "right", "l":
		m.adjustSetup(1)
	case "enter":
		return m.createRunFromSetup()
	}
	return nil
}

func (m *Model) createRunFromSetup() tea.Cmd {
	if m.inFlight {
		return nil
	}
	run, err := m.engine.NewRun(orchestrator.NewRunOptions{
		CWD:             m.setup.CWD,
		OutputRoot:      m.setup.OutputRoot,
		MaxRounds:       m.setup.MaxRounds,
		MergeStrategy:   m.setup.MergeMode,
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

	initialIntent := strings.TrimSpace(m.setupInput.Value())
	if initialIntent == "" {
		return nil
	}
	m.setupInput.SetValue("")
	m.inFlight = true
	go func(run model.RunState, text string) {
		updated, err := m.engine.UpdateBrief(context.Background(), run, text, func(snapshot model.RunState) {
			m.events <- snapshotMsg{Run: snapshot}
		})
		m.events <- briefDoneMsg{Run: updated, Err: err}
	}(m.run, initialIntent)
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
	case 6:
		if m.setup.MergeMode == model.MergeStrategyTogether {
			m.setup.MergeMode = model.MergeStrategySequential
		} else {
			m.setup.MergeMode = model.MergeStrategyTogether
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
	m.syncBriefViewportLayout()
	m.syncMonitorViewportLayout()
	m.syncResultsViewportLayout()
}

func (m Model) viewLoading() string {
	activity := m.motion.inlineWait(m.spin.View(), "Boot sequence", "Detecting local providers", m.chrome.muted)
	return joinBlocks(
		m.chrome.header("Panel of Experts", "Startup", ""),
		m.chrome.banner("Scanning", "Detecting providers and local auth state", toneInfo),
		m.chrome.muted.Render(activity),
	)
}

func (m Model) viewSetup() string {
	configPanel := m.chrome.panelBlock("Run Blueprint", strings.Join(m.setupConfigLines(), "\n"), m.setupConfigWidth(), toneFocus)
	providerPanel := m.chrome.panelBlock("Provider Status", m.providerStatusContent(), m.setupProviderWidth(), toneSecondary)
	intentPanel := m.chrome.panelBlock("Initial Intent", strings.Join([]string{
		m.setupInput.View(),
		m.chrome.muted.Render("Create the run and this message will be sent to the manager immediately."),
	}, "\n"), m.layout.contentWidth, toneInfo)

	var top string
	if m.setupUsesSplitPanels() {
		top = lipgloss.JoinHorizontal(lipgloss.Top, configPanel, strings.Repeat(" ", monitorPanelGap), providerPanel)
	} else {
		top = lipgloss.JoinVertical(lipgloss.Left, configPanel, "", providerPanel)
	}

	view := joinBlocks(
		m.chrome.header("Panel of Experts", "Setup", ""),
		m.chrome.banner("Ready", m.providerReadinessSummary(), toneInfo),
		top,
		intentPanel,
		m.chrome.muted.Render("Use tab/j/k to move, h/l to change values, enter to create the run. Focus Initial Intent to type your first request."),
	)
	if m.err != "" {
		view = joinBlocks(view, m.chrome.banner("Error", m.err, toneDanger))
	}
	return view
}

func (m Model) viewBrief() string {
	briefPanel := m.chrome.panelBlock("Brief Snapshot", m.briefViewport.View(), m.layout.contentWidth, toneInfo)
	blocks := []string{
		m.briefHeaderBlock(),
		briefPanel,
		m.briefFooterBlock(),
	}
	return joinBlocks(blocks...)
}

func (m Model) viewMonitor() string {
	statusPanel := m.chrome.panelBlock("Agent Status", m.statusViewport.View(), m.layout.monitorStatusW, toneInfo)
	timelinePanel := m.chrome.panelBlock("Timeline", m.timelineView.View(), m.layout.monitorTimelineW, toneWarning)

	var activity string
	if m.layout.monitorSplit {
		activity = lipgloss.JoinHorizontal(lipgloss.Top, statusPanel, strings.Repeat(" ", monitorPanelGap), timelinePanel)
	} else {
		activity = lipgloss.JoinVertical(lipgloss.Left, statusPanel, "", timelinePanel)
	}

	return joinBlocks(
		m.monitorHeaderBlock(),
		activity,
		m.monitorFooterBlock(),
	)
}

func (m Model) viewResults() string {
	resultsPanel := m.chrome.panelBlock("Final Markdown", m.resultViewport.View(), m.layout.contentWidth, toneSuccess)
	return joinBlocks(
		m.resultsTopBlock(),
		resultsPanel,
	)
}

func (m Model) setupConfigLines() []string {
	return []string{
		m.renderSetupField(0, "Manager", model.ProviderDisplayName(m.setup.Manager)),
		m.renderSetupField(1, "Expert 1", model.ProviderDisplayName(m.setup.Experts[0])),
		m.renderSetupField(2, "Expert 2", model.ProviderDisplayName(m.setup.Experts[1])),
		m.renderSetupField(3, "Expert 3", m.thirdExpertDisplay()),
		m.renderSetupField(4, "Expert count", fmt.Sprintf("%d", m.setup.ExpertCount)),
		m.renderSetupField(5, "Max rounds", fmt.Sprintf("%d", m.setup.MaxRounds)),
		m.renderSetupField(6, "Merge mode", model.MergeStrategyDisplayName(m.setup.MergeMode)),
		"",
		m.chrome.labeledLine("Workspace", m.setup.CWD),
		m.chrome.labeledLine("Output root", m.setup.OutputRoot),
	}
}

func (m Model) setupUsesSplitPanels() bool {
	return m.width >= 110
}

func (m Model) setupConfigWidth() int {
	if !m.setupUsesSplitPanels() {
		return m.layout.contentWidth
	}
	return max(minPanelWidth, (m.layout.contentWidth-monitorPanelGap)/2)
}

func (m Model) setupProviderWidth() int {
	if !m.setupUsesSplitPanels() {
		return m.layout.contentWidth
	}
	return max(minPanelWidth, m.layout.contentWidth-m.setupConfigWidth()-monitorPanelGap)
}

func (m Model) providerReadinessSummary() string {
	ready, available, missing := 0, 0, 0
	for _, id := range model.AllProviders() {
		status := m.capabilityStatus(m.capabilities[id])
		switch status {
		case "ready":
			ready++
		case "available":
			available++
		default:
			missing++
		}
	}
	return fmt.Sprintf("%d ready, %d available, %d missing", ready, available, missing)
}

func (m Model) capabilityStatus(capability model.Capability) string {
	if capability.Available {
		if capability.Authenticated {
			return "ready"
		}
		return "available"
	}
	return "missing"
}

func (m Model) providerStatusContent() string {
	lines := make([]string, 0, len(model.AllProviders())*2)
	for _, id := range model.AllProviders() {
		capability := m.capabilities[id]
		status := m.capabilityStatus(capability)
		lines = append(lines,
			lipgloss.JoinHorizontal(
				lipgloss.Center,
				m.chrome.providerBadge(id),
				" ",
				m.chrome.statusBadge(status),
			),
			"  "+emptyFallback(capability.Summary, "No provider summary available"),
		)
	}
	return strings.Join(lines, "\n\n")
}

func (m Model) briefHeaderBlock() string {
	meta := lipgloss.JoinHorizontal(
		lipgloss.Center,
		m.chrome.metaBadge("Run", m.run.ID, toneSecondary),
		" ",
		m.chrome.metaBadge("Status", humanizeToken(string(m.run.Status)), statusTone(string(m.run.Status))),
	)
	waiting := m.chrome.labeledLine("Waiting", emptyFallback(m.run.WaitingSummary, "Waiting for the next user action"))
	return joinBlocks(
		m.chrome.header(m.run.ProjectTitle, "Manager Brief", m.inlineActivity("Brief live")),
		meta,
		waiting,
	)
}

func (m Model) briefFooterBlock() string {
	blocks := []string{}
	if question, index, total := m.activeBriefQuestion(); question != "" {
		blocks = append(blocks, m.chrome.panelBlock("Manager Question", strings.Join([]string{
			fmt.Sprintf("Manager Question %d of %d", index+1, total),
			"",
			question,
		}, "\n"), m.layout.contentWidth, toneWarning))
	}

	if m.inFlight {
		blocks = append(blocks, m.chrome.banner("Working", "Manager is updating the brief", toneWarning))
	} else if _, index, total := m.activeBriefQuestion(); total > 0 {
		blocks = append(blocks, m.chrome.muted.Render(fmt.Sprintf("Enter submits your answer to manager question %d of %d. Press ctrl+s to start the discussion anyway.", index+1, total)))
	} else {
		blocks = append(blocks, m.chrome.muted.Render("Enter sends the next message to the manager. Press ctrl+s to start the discussion."))
	}

	blocks = append(blocks, m.chrome.panelBlock("Reply", m.input.View(), m.layout.contentWidth, toneFocus))
	if m.err != "" {
		blocks = append(blocks, m.chrome.banner("Error", m.err, toneDanger))
	}
	return joinBlocks(blocks...)
}

func (m Model) monitorHeaderBlock() string {
	meta := lipgloss.JoinHorizontal(
		lipgloss.Center,
		m.chrome.metaBadge("Run", m.run.ID, toneSecondary),
		" ",
		m.chrome.metaBadge("Phase", humanizeToken(m.run.CurrentPhase), phaseTone(m.run.CurrentPhase)),
		" ",
		m.chrome.metaBadge("Round", m.run.DisplayRound(), toneInfo),
		" ",
		m.chrome.metaBadge("Status", humanizeToken(string(m.run.Status)), statusTone(string(m.run.Status))),
	)

	blocks := []string{
		m.chrome.header(m.run.ProjectTitle, "Discussion Monitor", m.inlineActivity("Discussion live")),
		meta,
		m.chrome.labeledLine("Waiting", emptyFallback(m.run.WaitingSummary, "Waiting for the next orchestration step")),
	}
	if failureSummary := m.currentFailureSummary(); failureSummary != "" {
		blocks = append(blocks, m.chrome.error.Render("Failure: "+failureSummary))
	}
	return joinBlocks(blocks...)
}

func (m Model) monitorFooterBlock() string {
	blocks := []string{}
	if m.run.FinalProposal != nil {
		blocks = append(blocks, m.chrome.banner("Ready", "Discussion finished. Press r to view the final markdown.", toneSuccess))
	} else if m.inFlight {
		blocks = append(blocks, m.chrome.banner("Running", "Orchestration is active", toneWarning))
	}
	if m.err != "" && m.err != m.currentFailureSummary() {
		blocks = append(blocks, m.chrome.banner("Error", m.err, toneDanger))
	}
	return joinBlocks(blocks...)
}

func (m Model) resultsTopBlock() string {
	meta := lipgloss.JoinHorizontal(
		lipgloss.Center,
		m.chrome.metaBadge("Run", m.run.ID, toneSecondary),
		" ",
		m.chrome.metaBadge("Stop", humanizeToken(string(m.run.StopReason)), statusTone(string(m.run.StopReason))),
		" ",
		m.chrome.metaBadge("Status", humanizeToken(string(m.run.Status)), statusTone(string(m.run.Status))),
	)

	savedLines := []string{
		m.chrome.labeledLine("Final markdown", emptyFallback(m.run.FinalMarkdownPath, "Not written")),
	}
	if strings.TrimSpace(m.run.DeliverablePath) != "" {
		savedLines = append(savedLines, m.chrome.labeledLine("Deliverable file", m.run.DeliverablePath))
	}

	blocks := []string{
		m.chrome.header(m.run.ProjectTitle, "Final Proposal", ""),
		meta,
		m.chrome.panelBlock("Saved Outputs", strings.Join(savedLines, "\n"), m.layout.contentWidth, toneInfo),
	}
	if failureSummary := m.currentFailureSummary(); failureSummary != "" {
		blocks = append(blocks, m.chrome.banner("Failure", failureSummary, toneDanger))
	}
	blocks = append(blocks,
		m.chrome.banner("Ready", "Final proposal ready. Review the markdown below or use the saved file paths above.", toneSuccess),
		m.chrome.muted.Render("Use up/down or j/k to scroll. Press m to return to the monitor, q to quit."),
	)
	if m.err != "" && m.err != m.currentFailureSummary() {
		blocks = append(blocks, m.chrome.banner("Error", m.err, toneDanger))
	}
	return joinBlocks(blocks...)
}

func (m Model) inlineActivity(label string) string {
	if !m.inFlight {
		return ""
	}
	return m.motion.inlineWait(m.spin.View(), label, "", m.chrome.muted)
}

func (m Model) renderSetupField(index int, label, value string) string {
	return m.chrome.setupField(label, value, m.setup.Focus == index)
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

func (m *Model) syncSetupInputFocus() {
	if m.setup.Focus == 7 {
		m.setupInput.Focus()
		return
	}
	m.setupInput.Blur()
}

func (m *Model) syncBriefViewportLayout() {
	if m.width == 0 || m.height == 0 || m.run.ID == "" || m.layout.contentWidth == 0 {
		return
	}
	headerHeight := lipgloss.Height(m.briefHeaderBlock())
	footerHeight := lipgloss.Height(m.briefFooterBlock())
	panelChrome := m.chrome.panelChromeHeight("Brief Snapshot", m.layout.contentWidth, toneInfo)
	m.briefViewport.SetHeight(m.layout.viewportHeight(headerHeight, footerHeight, panelChrome, 5, 1))
	m.briefViewport.SetContent(m.briefContent())
}

func (m *Model) syncMonitorViewportLayout() {
	if m.width == 0 || m.height == 0 || m.run.ID == "" || m.layout.contentWidth == 0 {
		return
	}

	m.statusViewport.SetWidth(m.layout.monitorStatusW)
	m.timelineView.SetWidth(m.layout.monitorTimelineW)

	headerHeight := lipgloss.Height(m.monitorHeaderBlock())
	footerHeight := lipgloss.Height(m.monitorFooterBlock())
	statusChrome := m.chrome.panelChromeHeight("Agent Status", m.layout.monitorStatusW, toneInfo)

	if m.layout.monitorSplit {
		height := m.layout.viewportHeight(headerHeight, footerHeight, statusChrome, 4, minTallViewportHeight)
		m.statusViewport.SetHeight(height)
		m.timelineView.SetHeight(height)
	} else {
		timelineChrome := m.chrome.panelChromeHeight("Timeline", m.layout.monitorTimelineW, toneWarning)
		statusHeight, timelineHeight := m.layout.stackedViewportHeights(headerHeight, footerHeight, max(statusChrome, timelineChrome), 5, minTallViewportHeight)
		m.statusViewport.SetHeight(statusHeight)
		m.timelineView.SetHeight(timelineHeight)
	}

	m.statusViewport.SetContent(m.statusContent())
	m.timelineView.SetContent(m.timelineContent())
	m.timelineView.GotoBottom()
}

func (m *Model) syncResultsViewportLayout() {
	if m.width == 0 || m.height == 0 || m.run.ID == "" || m.layout.contentWidth == 0 {
		return
	}
	headerHeight := lipgloss.Height(m.resultsTopBlock())
	panelChrome := m.chrome.panelChromeHeight("Final Markdown", m.layout.contentWidth, toneSuccess)
	m.resultViewport.SetHeight(m.layout.viewportHeight(headerHeight, 0, panelChrome, 4, minViewportHeight))
	m.resultViewport.SetContent(m.resultContent())
}

func (m Model) currentFailureSummary() string {
	if strings.TrimSpace(m.err) != "" {
		return strings.TrimSpace(m.err)
	}
	if strings.TrimSpace(m.run.FailureSummary) != "" {
		return strings.TrimSpace(m.run.FailureSummary)
	}
	if m.run.Status != model.RunStatusFailed {
		return ""
	}
	if strings.TrimSpace(m.run.WaitingSummary) != "" {
		return strings.TrimSpace(m.run.WaitingSummary)
	}
	if status, ok := m.run.AgentStatuses[m.run.Manager.ID]; ok && status.State == model.AgentStateError && strings.TrimSpace(status.Summary) != "" {
		return strings.TrimSpace(status.Summary)
	}
	for _, status := range orderedStatusesForView(m.run) {
		if status.State == model.AgentStateError && strings.TrimSpace(status.Summary) != "" {
			return strings.TrimSpace(status.Summary)
		}
	}
	if m.run.StopReason != "" {
		return string(m.run.StopReason)
	}
	return ""
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
	lines := []string{}
	for i, agent := range m.run.AllAgents() {
		status := m.run.AgentStatuses[agent.ID]
		lines = append(lines,
			lipgloss.JoinHorizontal(
				lipgloss.Center,
				m.chrome.value.Copy().Bold(true).Render(agent.Name),
				" ",
				m.chrome.providerBadge(agent.Provider),
				" ",
				m.chrome.stateBadge(status.State),
			),
			"  "+m.chrome.labeledLine("Step", emptyFallback(humanizeToken(status.LastStep), "No step yet")),
			"  "+m.chrome.labeledLine("Summary", emptyFallback(status.Summary, "No updates yet")),
		)
		if i < len(m.run.AllAgents())-1 {
			lines = append(lines, "", m.chrome.divider.Render(strings.Repeat("─", max(12, m.statusViewport.Width()-6))), "")
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func (m Model) timelineContent() string {
	if m.run.ID == "" {
		return ""
	}
	lines := []string{}
	for _, entry := range m.run.Timeline {
		text := strings.TrimSpace(entry.Text)
		if text == "" {
			continue
		}
		lines = append(lines, lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.chrome.muted.Render(entry.Timestamp.Format(timeFormat)),
			"  ",
			m.chrome.value.Render(text),
		))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
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

func orderedStatusesForView(run model.RunState) []model.AgentStatus {
	statuses := make([]model.AgentStatus, 0, len(run.AgentStatuses))
	if status, ok := run.AgentStatuses[run.Manager.ID]; ok {
		statuses = append(statuses, status)
	}
	for _, expert := range run.Experts {
		if status, ok := run.AgentStatuses[expert.ID]; ok {
			statuses = append(statuses, status)
		}
	}
	return statuses
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
	return appenv.WorkspaceOutputRoot(cwd)
}
