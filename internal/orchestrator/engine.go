package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"panelofexperts/internal/model"
	"panelofexperts/internal/providers"
	"panelofexperts/internal/render"
)

type SnapshotFn func(model.RunState)

type NewRunOptions struct {
	CWD             string
	OutputRoot      string
	MaxRounds       int
	ManagerProvider model.ProviderID
	ExpertProviders []model.ProviderID
}

type Engine struct {
	providers map[model.ProviderID]providers.AgentProvider
	now       func() time.Time
}

const (
	managerBriefTimeout    = 5 * time.Minute
	managerProposalTimeout = 5 * time.Minute
	expertReviewTimeout    = 4 * time.Minute
)

func NewEngine(providersList ...providers.AgentProvider) *Engine {
	providerMap := make(map[model.ProviderID]providers.AgentProvider, len(providersList))
	for _, provider := range providersList {
		providerMap[provider.ID()] = provider
	}
	return &Engine{
		providers: providerMap,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (e *Engine) DetectCapabilities(ctx context.Context) map[model.ProviderID]model.Capability {
	results := make(map[model.ProviderID]model.Capability, len(e.providers))
	for _, providerID := range model.AllProviders() {
		provider, ok := e.providers[providerID]
		if !ok {
			results[providerID] = model.Capability{
				Provider:    providerID,
				DisplayName: model.ProviderDisplayName(providerID),
				Summary:     "Provider not configured",
			}
			continue
		}
		results[providerID] = provider.Detect(ctx)
	}
	return results
}

func (e *Engine) NewRun(options NewRunOptions) (model.RunState, error) {
	if options.CWD == "" {
		options.CWD = "."
	}
	if options.MaxRounds <= 0 {
		options.MaxRounds = 10
	}
	absCWD, err := filepath.Abs(options.CWD)
	if err != nil {
		return model.RunState{}, err
	}
	if options.OutputRoot == "" {
		options.OutputRoot = filepath.Join(absCWD, ".panel-of-experts", "runs")
	}
	absOutputRoot, err := filepath.Abs(options.OutputRoot)
	if err != nil {
		return model.RunState{}, err
	}

	runID := fmt.Sprintf("%s-%s", e.now().Format("20060102-150405"), slugify(filepath.Base(absCWD)))
	runDir := filepath.Join(absOutputRoot, runID)
	if _, err := NewStore(runDir); err != nil {
		return model.RunState{}, err
	}

	manager := model.AgentConfig{
		ID:       "manager",
		Name:     fmt.Sprintf("Manager (%s)", model.ProviderDisplayName(options.ManagerProvider)),
		Role:     model.RoleManager,
		Provider: options.ManagerProvider,
	}

	lenses := model.DefaultLenses(len(options.ExpertProviders))
	experts := make([]model.AgentConfig, 0, len(options.ExpertProviders))
	for i, providerID := range options.ExpertProviders {
		lens := model.LensExecution
		if i < len(lenses) {
			lens = lenses[i]
		}
		experts = append(experts, model.AgentConfig{
			ID:       fmt.Sprintf("expert-%d", i+1),
			Name:     fmt.Sprintf("Expert %d (%s)", i+1, model.ProviderDisplayName(providerID)),
			Role:     model.RoleExpert,
			Provider: providerID,
			Lens:     lens,
		})
	}

	run := model.NewRunState(runID, absCWD, runDir, options.MaxRounds, manager, experts)
	store, err := NewStore(run.OutputDir)
	if err != nil {
		return model.RunState{}, err
	}
	if err := store.SaveState(run); err != nil {
		return model.RunState{}, err
	}
	return run, nil
}

func (e *Engine) UpdateBrief(ctx context.Context, run model.RunState, userMessage string, onSnapshot SnapshotFn) (model.RunState, error) {
	provider, err := e.getProvider(run.Manager.Provider)
	if err != nil {
		return run, err
	}
	store, err := NewStore(run.OutputDir)
	if err != nil {
		return run, err
	}

	var mu sync.Mutex
	notify := func() {
		if onSnapshot == nil {
			return
		}
		onSnapshot(run.Clone())
	}
	updateProgress := func(event model.ProgressEvent) {
		mu.Lock()
		defer mu.Unlock()
		e.applyProgress(&run, event)
		_ = store.AppendEvent(event)
		_ = store.SaveState(run)
		notify()
	}

	run.Status = model.RunStatusRunning
	run.CurrentPhase = "manager_brief"
	run.StopReason = model.StopReasonAwaitingUser
	run.WaitingSummary = "Waiting on manager brief update"
	e.touch(&run)
	notify()

	progressCh := make(chan model.ProgressEvent, 32)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for event := range progressCh {
			updateProgress(event)
		}
	}()

	timeoutCtx, cancel := context.WithTimeout(ctx, managerBriefTimeout)
	defer cancel()
	result, runErr := provider.Run(timeoutCtx, model.Request{
		RunID:      run.ID,
		Round:      0,
		Version:    0,
		AgentID:    run.Manager.ID,
		Role:       model.RoleManager,
		CWD:        run.CWD,
		Prompt:     buildBriefPrompt(run, userMessage),
		JSONSchema: briefSchema,
		OutputKind: "brief",
		Timeout:    managerBriefTimeout,
		Metadata: map[string]string{
			"workspace_access": "none",
		},
	}, progressCh)
	close(progressCh)
	wg.Wait()
	if runErr != nil {
		run.Status = model.RunStatusFailed
		run.CurrentPhase = "manager_brief_failed"
		run.StopReason = model.StopReasonManagerFailed
		run.WaitingSummary = ""
		run.AgentStatuses[run.Manager.ID] = updateAgentState(run.AgentStatuses[run.Manager.ID], model.AgentStateError, "brief_failed", summarizeError(runErr), e.now())
		e.touch(&run)
		_ = store.SaveState(run)
		notify()
		return run, runErr
	}

	brief, err := parseProviderOutput[model.Brief](run.Manager.Provider, result.Stdout)
	if err != nil {
		run.Status = model.RunStatusFailed
		run.CurrentPhase = "manager_brief_failed"
		run.StopReason = model.StopReasonManagerFailed
		run.WaitingSummary = ""
		run.AgentStatuses[run.Manager.ID] = updateAgentState(run.AgentStatuses[run.Manager.ID], model.AgentStateError, "brief_failed", summarizeError(err), e.now())
		e.touch(&run)
		_ = store.SaveState(run)
		notify()
		return run, err
	}

	run.Brief = brief
	if strings.TrimSpace(brief.ProjectTitle) != "" {
		run.ProjectTitle = strings.TrimSpace(brief.ProjectTitle)
	}
	run.ManagerTurns = append(run.ManagerTurns, model.ManagerTurn{
		Timestamp:    e.now(),
		UserMessage:  strings.TrimSpace(userMessage),
		BriefSummary: strings.TrimSpace(brief.IntentSummary),
	})
	run.AgentStatuses[run.Manager.ID] = updateAgentState(run.AgentStatuses[run.Manager.ID], model.AgentStateDone, "brief_ready", "Manager updated the brief", e.now())
	run.Status = model.RunStatusWaiting
	run.CurrentPhase = "brief_ready"
	run.WaitingSummary = "Waiting for the user to start the discussion"
	e.appendTimeline(&run, 0, run.Manager.ID, "Manager updated the brief")
	e.touch(&run)
	_ = store.SaveJSON("brief.json", run.Brief)
	_ = store.SaveText("brief.md", render.RenderBriefMarkdown(run.Brief))
	_ = store.SaveState(run)
	notify()
	return run, nil
}

func (e *Engine) RunDiscussion(ctx context.Context, run model.RunState, onSnapshot SnapshotFn) (model.RunState, error) {
	store, err := NewStore(run.OutputDir)
	if err != nil {
		return run, err
	}
	managerProvider, err := e.getProvider(run.Manager.Provider)
	if err != nil {
		return run, err
	}

	var mu sync.Mutex
	notify := func() {
		if onSnapshot == nil {
			return
		}
		onSnapshot(run.Clone())
	}
	updateProgress := func(event model.ProgressEvent) {
		mu.Lock()
		defer mu.Unlock()
		e.applyProgress(&run, event)
		_ = store.AppendEvent(event)
		_ = store.SaveState(run)
		notify()
	}

	run.Status = model.RunStatusRunning
	run.CurrentPhase = "manager_initial_proposal"
	run.WaitingSummary = "Waiting on manager initial proposal"
	run.CurrentRound = 1
	e.touch(&run)
	notify()

	proposal, version, err := e.runManagerProposal(ctx, managerProvider, &run, store, updateProgress, buildInitialProposalPrompt(run), 1)
	if err != nil {
		run.Status = model.RunStatusFailed
		run.CurrentPhase = "manager_initial_proposal_failed"
		run.StopReason = model.StopReasonManagerFailed
		run.WaitingSummary = ""
		e.touch(&run)
		_ = store.SaveState(run)
		notify()
		return run, err
	}

	stopReason := model.StopReasonDiscussionEnded
	for round := 1; round <= run.MaxRounds; round++ {
		run.CurrentRound = round
		run.CurrentPhase = "expert_reviews"
		run.WaitingSummary = "Waiting on expert reviews"
		run.Status = model.RunStatusRunning
		run.AgentStatuses[run.Manager.ID] = updateAgentState(run.AgentStatuses[run.Manager.ID], model.AgentStateWaitingOnExperts, "expert_reviews", "Waiting on expert reviews", e.now())
		e.touch(&run)
		notify()

		roundState := model.RoundState{
			Round:           round,
			ProposalVersion: version,
			Proposal:        proposal,
			StartedAt:       e.now(),
		}

		reviews, err := e.collectExpertReviews(ctx, &run, store, updateProgress, round, proposal)
		if err != nil {
			run.Status = model.RunStatusFailed
			run.CurrentPhase = "expert_reviews_failed"
			run.StopReason = model.StopReasonExpertsFailed
			run.WaitingSummary = ""
			e.touch(&run)
			_ = store.SaveState(run)
			notify()
			return run, err
		}
		for _, review := range reviews {
			roundState.ExpertReviews = append(roundState.ExpertReviews, review)
		}

		previousHash := render.ProposalHash(proposal)
		allNoChanges := true
		for _, expert := range run.Experts {
			review, ok := reviewsByLensOrAgent(expert, reviews)
			if !ok {
				continue
			}
			if review.RequiresChanges {
				allNoChanges = false
			}
			run.CurrentPhase = "manager_merge"
			run.WaitingSummary = fmt.Sprintf("Waiting on manager merge for %s", expert.Name)
			run.AgentStatuses[expert.ID] = updateAgentState(run.AgentStatuses[expert.ID], model.AgentStateWaitingOnManager, "manager_merge", "Waiting on manager merge", e.now())
			e.touch(&run)
			notify()

			merged, nextVersion, mergeErr := e.runManagerProposal(
				ctx,
				managerProvider,
				&run,
				store,
				updateProgress,
				buildMergePrompt(run, proposal, review, expert),
				version+1,
			)
			if mergeErr != nil {
				run.Status = model.RunStatusFailed
				run.CurrentPhase = "manager_merge_failed"
				run.StopReason = model.StopReasonManagerFailed
				run.WaitingSummary = ""
				e.touch(&run)
				_ = store.SaveState(run)
				notify()
				return run, mergeErr
			}
			proposal = merged
			version = nextVersion
			roundState.ProposalVersion = version
			roundState.Proposal = proposal
		}

		now := e.now()
		roundState.CompletedAt = &now
		run.Rounds = append(run.Rounds, roundState)
		e.touch(&run)
		_ = store.SaveState(run)
		notify()

		newHash := render.ProposalHash(proposal)
		switch {
		case proposal.Converged:
			stopReason = model.StopReasonConverged
			run.Status = model.RunStatusConverged
			goto finalize
		case previousHash == newHash:
			stopReason = model.StopReasonProposalStable
			run.Status = model.RunStatusConverged
			goto finalize
		case allNoChanges:
			stopReason = model.StopReasonConverged
			run.Status = model.RunStatusConverged
			goto finalize
		}
	}

	stopReason = model.StopReasonMaxRounds
	run.Status = model.RunStatusComplete

finalize:
	run.StopReason = stopReason
	run.FinalProposal = &proposal
	run.FinalMarkdown = render.RenderProposalMarkdown(proposal, run)
	run.FinalMarkdownPath = filepath.Join(run.OutputDir, "final.md")
	run.CurrentPhase = "finalized"
	run.WaitingSummary = ""
	if run.Status != model.RunStatusConverged {
		run.Status = model.RunStatusComplete
	}
	e.appendTimeline(&run, run.CurrentRound, run.Manager.ID, "Manager finalized the discussion")
	e.touch(&run)
	_ = store.SaveText("final.md", run.FinalMarkdown)
	_ = store.SaveState(run)
	notify()
	return run, nil
}

func (e *Engine) collectExpertReviews(
	ctx context.Context,
	run *model.RunState,
	store *Store,
	updateProgress func(model.ProgressEvent),
	round int,
	proposal model.Proposal,
) ([]model.ExpertReview, error) {
	type outcome struct {
		index  int
		agent  model.AgentConfig
		review model.ExpertReview
		err    error
	}

	outcomes := make(chan outcome, len(run.Experts))
	for i, expert := range run.Experts {
		go func(index int, agent model.AgentConfig) {
			provider, err := e.getProvider(agent.Provider)
			if err != nil {
				outcomes <- outcome{index: index, agent: agent, err: err}
				return
			}

			progressCh := make(chan model.ProgressEvent, 16)
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				for event := range progressCh {
					updateProgress(event)
				}
			}()

			timeoutCtx, cancel := context.WithTimeout(ctx, expertReviewTimeout)
			defer cancel()
			result, runErr := provider.Run(timeoutCtx, model.Request{
				RunID:      run.ID,
				Round:      round,
				Version:    run.LatestProposalVersion(),
				AgentID:    agent.ID,
				Role:       agent.Role,
				Lens:       agent.Lens,
				CWD:        run.CWD,
				Prompt:     buildExpertReviewPrompt(*run, proposal, agent),
				JSONSchema: reviewSchema,
				OutputKind: "review",
				Timeout:    expertReviewTimeout,
			}, progressCh)
			close(progressCh)
			wg.Wait()
			if runErr != nil {
				outcomes <- outcome{index: index, agent: agent, err: runErr}
				return
			}
			review, parseErr := parseProviderOutput[model.ExpertReview](agent.Provider, result.Stdout)
			if parseErr != nil {
				outcomes <- outcome{index: index, agent: agent, err: parseErr}
				return
			}
			review.Lens = agent.Lens
			_ = store.SaveJSON(filepath.Join("reviews", fmt.Sprintf("round-%d", round), agent.ID+".json"), review)
			outcomes <- outcome{index: index, agent: agent, review: review}
		}(i, expert)
	}

	results := make([]outcome, 0, len(run.Experts))
	successes := 0
	for range run.Experts {
		result := <-outcomes
		results = append(results, result)
		if result.err == nil {
			successes++
			e.appendTimeline(run, round, result.agent.ID, fmt.Sprintf("%s returned a %s review", result.agent.Name, result.agent.Lens))
		} else {
			run.AgentStatuses[result.agent.ID] = updateAgentState(run.AgentStatuses[result.agent.ID], model.AgentStateError, "review_failed", summarizeError(result.err), e.now())
			e.appendTimeline(run, round, result.agent.ID, fmt.Sprintf("%s failed: %s", result.agent.Name, summarizeError(result.err)))
		}
	}
	if successes == 0 {
		return nil, errors.New("all expert reviews failed")
	}

	slices.SortFunc(results, func(a, b outcome) int {
		return a.index - b.index
	})
	reviews := make([]model.ExpertReview, 0, successes)
	for _, result := range results {
		if result.err != nil {
			continue
		}
		reviews = append(reviews, result.review)
		run.AgentStatuses[result.agent.ID] = updateAgentState(run.AgentStatuses[result.agent.ID], model.AgentStateDone, "review_complete", result.review.Summary, e.now())
	}
	return reviews, nil
}

func (e *Engine) runManagerProposal(
	ctx context.Context,
	provider providers.AgentProvider,
	run *model.RunState,
	store *Store,
	updateProgress func(model.ProgressEvent),
	prompt string,
	version int,
) (model.Proposal, int, error) {
	progressCh := make(chan model.ProgressEvent, 16)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for event := range progressCh {
			updateProgress(event)
		}
	}()

	timeoutCtx, cancel := context.WithTimeout(ctx, managerProposalTimeout)
	defer cancel()
	result, err := provider.Run(timeoutCtx, model.Request{
		RunID:      run.ID,
		Round:      run.CurrentRound,
		Version:    version,
		AgentID:    run.Manager.ID,
		Role:       run.Manager.Role,
		CWD:        run.CWD,
		Prompt:     prompt,
		JSONSchema: proposalSchema,
		OutputKind: "proposal",
		Timeout:    managerProposalTimeout,
	}, progressCh)
	close(progressCh)
	wg.Wait()
	if err != nil {
		return model.Proposal{}, version, err
	}

	proposal, err := parseProviderOutput[model.Proposal](run.Manager.Provider, result.Stdout)
	if err != nil {
		return model.Proposal{}, version, err
	}

	filename := fmt.Sprintf("proposal-v%03d", version)
	_ = store.SaveJSON(filename+".json", proposal)
	tempRun := run.Clone()
	tempRun.FinalProposal = &proposal
	tempRun.StopReason = model.StopReasonDiscussionEnded
	_ = store.SaveText(filename+".md", render.RenderProposalMarkdown(proposal, tempRun))
	e.appendTimeline(run, run.CurrentRound, run.Manager.ID, fmt.Sprintf("Manager drafted proposal v%03d", version))
	run.AgentStatuses[run.Manager.ID] = updateAgentState(run.AgentStatuses[run.Manager.ID], model.AgentStateDone, "proposal_complete", proposal.ChangeSummary, e.now())
	e.touch(run)
	_ = store.SaveState(*run)
	return proposal, version, nil
}

func (e *Engine) getProvider(providerID model.ProviderID) (providers.AgentProvider, error) {
	provider, ok := e.providers[providerID]
	if !ok {
		return nil, fmt.Errorf("provider %s is not configured", providerID)
	}
	return provider, nil
}

func (e *Engine) applyProgress(run *model.RunState, event model.ProgressEvent) {
	status, ok := run.AgentStatuses[event.AgentID]
	if !ok {
		return
	}
	run.AgentStatuses[event.AgentID] = updateAgentState(status, event.State, event.Step, event.Summary, event.Timestamp)
	if event.Step != "" || event.Summary != "" {
		e.appendTimeline(run, event.Round, event.AgentID, formatTimelineText(event))
	}
	run.WaitingSummary = deriveWaitingSummary(*run)
	e.touch(run)
}

func updateAgentState(status model.AgentStatus, state model.AgentState, step, summary string, ts time.Time) model.AgentStatus {
	status.State = state
	if strings.TrimSpace(step) != "" {
		status.LastStep = step
	}
	if strings.TrimSpace(summary) != "" {
		status.Summary = strings.TrimSpace(summary)
	}
	status.UpdatedAt = ts
	return status
}

func deriveWaitingSummary(run model.RunState) string {
	switch run.Status {
	case model.RunStatusConverged:
		return "Discussion converged"
	case model.RunStatusComplete:
		return "Discussion complete"
	case model.RunStatusFailed:
		return ""
	}

	active := []string{}
	waitingExperts := []string{}
	waitingManager := false
	for _, status := range run.AgentStatuses {
		switch status.State {
		case model.AgentStateQueued, model.AgentStateStarting, model.AgentStateRunning, model.AgentStateParsing:
			active = append(active, status.Name)
		case model.AgentStateWaitingOnExperts:
			waitingExperts = append(waitingExperts, status.Name)
		case model.AgentStateWaitingOnManager:
			waitingManager = true
		}
	}
	if len(active) > 0 {
		return "Waiting on " + strings.Join(active, ", ")
	}
	if len(waitingExperts) > 0 {
		return "Waiting on expert reviews"
	}
	if waitingManager {
		return "Waiting on manager merge"
	}
	switch run.Status {
	case model.RunStatusWaiting:
		return "Waiting for the next user action"
	default:
		return run.WaitingSummary
	}
}

func formatTimelineText(event model.ProgressEvent) string {
	if strings.TrimSpace(event.Summary) == "" {
		return event.Step
	}
	return fmt.Sprintf("%s: %s", model.ProviderDisplayName(event.Provider), event.Summary)
}

func (e *Engine) appendTimeline(run *model.RunState, round int, agentID, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	run.Timeline = append(run.Timeline, model.TimelineEntry{
		Timestamp: e.now(),
		Round:     round,
		AgentID:   agentID,
		Text:      text,
	})
}

func (e *Engine) touch(run *model.RunState) {
	run.WaitingSummary = deriveWaitingSummary(*run)
	run.UpdatedAt = e.now()
}

func parseProviderOutput[T any](provider model.ProviderID, raw string) (T, error) {
	var zero T

	var wrapped struct {
		Response         json.RawMessage `json:"response"`
		StructuredOutput json.RawMessage `json:"structured_output"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &wrapped); err == nil {
		if len(wrapped.StructuredOutput) > 0 {
			if value, err := decodeDirect[T](string(wrapped.StructuredOutput)); err == nil {
				return value, nil
			}
		}
		if len(wrapped.Response) > 0 && wrapped.Response[0] == '"' {
			var inner string
			if err := json.Unmarshal(wrapped.Response, &inner); err == nil {
				return decodeDirect[T](inner)
			}
		}
		if len(wrapped.Response) > 0 {
			if value, err := decodeDirect[T](string(wrapped.Response)); err == nil {
				return value, nil
			}
		}
	}
	if value, err := decodeDirect[T](raw); err == nil {
		return value, nil
	}
	return zero, fmt.Errorf("unable to parse %s response as %T", provider, zero)
}

func decodeDirect[T any](raw string) (T, error) {
	var zero T
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return zero, errors.New("empty output")
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		raw = raw[start : end+1]
	}
	var value T
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return zero, err
	}
	return value, nil
}

func summarizeError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if len(msg) > 96 {
		return msg[:93] + "..."
	}
	return msg
}

func reviewsByLensOrAgent(agent model.AgentConfig, reviews []model.ExpertReview) (model.ExpertReview, bool) {
	for _, review := range reviews {
		if review.Lens == agent.Lens {
			return review, true
		}
	}
	return model.ExpertReview{}, false
}

func slugify(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return "run"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "run"
	}
	return out
}
