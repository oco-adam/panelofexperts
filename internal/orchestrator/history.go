package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"panelofexperts/internal/model"
	"panelofexperts/internal/render"
)

type ResumeRunOptions struct {
	DeliverableTimeout time.Duration
}

type RerunOptions struct {
	OutputRoot         string
	MaxRounds          int
	MergeStrategy      model.MergeStrategy
	DeliverableTimeout time.Duration
	ManagerProvider    model.ProviderID
	ExpertProviders    []model.ProviderID
}

func ListRuns(outputRoot string) ([]model.RunState, error) {
	if strings.TrimSpace(outputRoot) == "" {
		return nil, errors.New("output root is required")
	}
	root, err := filepath.Abs(outputRoot)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []model.RunState{}, nil
		}
		return nil, err
	}

	runs := make([]model.RunState, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		run, err := LoadRun(entry.Name(), root)
		if err != nil {
			continue
		}
		runs = append(runs, run)
	}

	sort.Slice(runs, func(i, j int) bool {
		if runs[i].UpdatedAt.Equal(runs[j].UpdatedAt) {
			return runs[i].ID > runs[j].ID
		}
		return runs[i].UpdatedAt.After(runs[j].UpdatedAt)
	})
	return runs, nil
}

func LoadRun(runRef, outputRoot string) (model.RunState, error) {
	path, err := resolveRunStatePath(runRef, outputRoot)
	if err != nil {
		return model.RunState{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return model.RunState{}, err
	}
	var run model.RunState
	if err := json.Unmarshal(data, &run); err != nil {
		return model.RunState{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return run, nil
}

func CanResumeRun(run model.RunState) bool {
	return canResumeDeliverable(run)
}

func (e *Engine) ResumeRun(ctx context.Context, run model.RunState, options ResumeRunOptions, onSnapshot SnapshotFn) (model.RunState, error) {
	if options.DeliverableTimeout > 0 {
		run.DeliverableTimeout = options.DeliverableTimeout
	} else if run.DeliverableTimeout <= 0 {
		run.DeliverableTimeout = managerDeliverableTimeout
	}

	if !canResumeDeliverable(run) {
		return run, fmt.Errorf("run %s cannot be resumed from phase %s", run.ID, emptyFallback(run.CurrentPhase, "unknown"))
	}
	return e.resumeDeliverable(ctx, run, onSnapshot)
}

func (e *Engine) RerunFromRun(ctx context.Context, source model.RunState, options RerunOptions, onSnapshot SnapshotFn) (model.RunState, error) {
	if strings.TrimSpace(source.ID) == "" {
		return model.RunState{}, errors.New("source run is missing an id")
	}
	if strings.TrimSpace(source.CWD) == "" {
		return model.RunState{}, errors.New("source run is missing its workspace path")
	}
	if strings.TrimSpace(source.Brief.IntentSummary) == "" && len(source.Brief.Goals) == 0 {
		return model.RunState{}, fmt.Errorf("run %s does not contain a reusable brief", source.ID)
	}
	if !source.Brief.ReadyToStart {
		return model.RunState{}, fmt.Errorf("run %s brief is not ready to start", source.ID)
	}

	managerProvider := source.Manager.Provider
	if options.ManagerProvider != "" {
		managerProvider = options.ManagerProvider
	}

	expertProviders := make([]model.ProviderID, 0, len(source.Experts))
	if len(options.ExpertProviders) > 0 {
		expertProviders = append(expertProviders, options.ExpertProviders...)
	} else {
		for _, expert := range source.Experts {
			expertProviders = append(expertProviders, expert.Provider)
		}
	}

	maxRounds := source.MaxRounds
	if options.MaxRounds > 0 {
		maxRounds = options.MaxRounds
	}

	mergeStrategy := source.MergeStrategy
	if options.MergeStrategy != "" {
		mergeStrategy = options.MergeStrategy
	}

	deliverableTimeout := source.DeliverableTimeout
	if options.DeliverableTimeout > 0 {
		deliverableTimeout = options.DeliverableTimeout
	}
	outputRoot := strings.TrimSpace(options.OutputRoot)
	if outputRoot == "" && strings.TrimSpace(source.OutputDir) != "" {
		outputRoot = filepath.Dir(source.OutputDir)
	}

	run, err := e.NewRun(NewRunOptions{
		CWD:                source.CWD,
		OutputRoot:         outputRoot,
		MaxRounds:          maxRounds,
		MergeStrategy:      mergeStrategy,
		DeliverableTimeout: deliverableTimeout,
		ManagerProvider:    managerProvider,
		ExpertProviders:    expertProviders,
	})
	if err != nil {
		return model.RunState{}, err
	}

	store, err := NewStore(run.OutputDir)
	if err != nil {
		return model.RunState{}, err
	}

	run.Brief = source.Brief
	run.ManagerTurns = append([]model.ManagerTurn{}, source.ManagerTurns...)
	run.Status = model.RunStatusWaiting
	run.CurrentPhase = "brief_ready"
	run.StopReason = model.StopReasonAwaitingUser
	run.FailureSummary = ""
	run.WaitingSummary = "Loaded brief from a prior run and waiting to start the discussion"
	if title := strings.TrimSpace(run.Brief.ProjectTitle); title != "" {
		run.ProjectTitle = title
	}
	run.AgentStatuses[run.Manager.ID] = updateAgentState(
		run.AgentStatuses[run.Manager.ID],
		model.AgentStateDone,
		"brief_loaded",
		fmt.Sprintf("Loaded brief from prior run %s", source.ID),
		e.now(),
	)
	e.appendTimeline(&run, 0, run.Manager.ID, fmt.Sprintf("Loaded brief from prior run %s", source.ID))
	e.touch(&run)
	_ = store.SaveJSON("brief.json", run.Brief)
	_ = store.SaveText("brief.md", render.RenderBriefMarkdown(run.Brief))
	_ = store.SaveState(run)
	if onSnapshot != nil {
		onSnapshot(run.Clone())
	}

	return e.RunDiscussion(ctx, run, onSnapshot)
}

func canResumeDeliverable(run model.RunState) bool {
	if run.Brief.TaskKind != model.TaskKindDocument {
		return false
	}
	if run.FinalProposal == nil && run.LatestProposal() == nil && run.LatestDocumentDraft() == nil {
		return false
	}
	switch strings.TrimSpace(run.CurrentPhase) {
	case "writing_deliverable", "deliverable_draft_failed", "deliverable_write_failed":
		return true
	default:
		return false
	}
}

func (e *Engine) resumeDeliverable(ctx context.Context, run model.RunState, onSnapshot SnapshotFn) (model.RunState, error) {
	store, err := NewStore(run.OutputDir)
	if err != nil {
		return run, err
	}

	if latestDocument := run.LatestDocumentDraft(); latestDocument != nil && strings.TrimSpace(run.CurrentPhase) != "deliverable_draft_failed" {
		return e.resumeDocumentWrite(run, store, *latestDocument, onSnapshot)
	}

	managerProvider, err := e.getProvider(run.Manager.Provider)
	if err != nil {
		return run, err
	}

	proposal := run.FinalProposal
	if proposal == nil {
		latest := run.LatestProposal()
		if latest == nil {
			return run, errors.New("run does not contain a final proposal to resume from")
		}
		proposal = latest
	}
	run.FinalProposal = proposal

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
	run.CurrentPhase = "writing_deliverable"
	run.StopReason = model.StopReasonDiscussionEnded
	run.FailureSummary = ""
	run.WaitingSummary = "Waiting on final deliverable draft"
	run.AgentStatuses[run.Manager.ID] = updateAgentState(
		run.AgentStatuses[run.Manager.ID],
		model.AgentStateRunning,
		"writing_deliverable",
		fmt.Sprintf("Retrying final deliverable with timeout %s", deliverableTimeoutFor(run)),
		e.now(),
	)
	e.appendTimeline(&run, run.CurrentRound, run.Manager.ID, "Resuming final deliverable phase")
	e.touch(&run)
	_ = store.SaveState(run)
	notify()

	draft, draftErr := e.resolveDocumentDraft(
		ctx,
		managerProvider,
		&run,
		*proposal,
		updateProgress,
		model.DocumentDraft{
			Path:     run.DeliverablePath,
			Markdown: run.FinalMarkdown,
		},
	)
	if draftErr != nil {
		e.markRunFailed(&run, run.Manager.ID, "deliverable_draft_failed", model.StopReasonManagerFailed, "deliverable_draft_failed", draftErr)
		e.touch(&run)
		_ = store.SaveState(run)
		notify()
		return run, draftErr
	}

	run.DeliverablePath = draft.Path
	run.FinalMarkdown = strings.TrimSpace(draft.Markdown) + "\n"
	if err := writeDeliverableFile(run.DeliverablePath, run.FinalMarkdown); err != nil {
		e.markRunFailed(&run, run.Manager.ID, "deliverable_write_failed", model.StopReasonManagerFailed, "deliverable_write_failed", err)
		e.touch(&run)
		_ = store.SaveState(run)
		notify()
		return run, err
	}

	_ = store.SaveJSON("deliverable.json", model.DocumentDraft{
		Path:     run.DeliverablePath,
		Markdown: strings.TrimSpace(run.FinalMarkdown),
	})
	_ = store.SaveText("deliverable.md", run.FinalMarkdown)
	run.AgentStatuses[run.Manager.ID] = updateAgentState(
		run.AgentStatuses[run.Manager.ID],
		model.AgentStateDone,
		"deliverable_written",
		"Manager finalized the deliverable",
		e.now(),
	)
	e.appendTimeline(&run, run.CurrentRound, run.Manager.ID, fmt.Sprintf("Wrote deliverable to %s", run.DeliverablePath))

	status, stopReason := resumeOutcome(run, *proposal)
	run.PendingStatus = ""
	run.PendingStopReason = ""
	run.Status = status
	run.StopReason = stopReason
	run.FinalMarkdownPath = filepath.Join(run.OutputDir, "final.md")
	run.CurrentPhase = "finalized"
	run.WaitingSummary = ""
	e.appendTimeline(&run, run.CurrentRound, run.Manager.ID, "Manager finalized the discussion")
	e.touch(&run)
	_ = store.SaveText("final.md", run.FinalMarkdown)
	_ = store.SaveState(run)
	notify()
	return run, nil
}

func (e *Engine) resumeDocumentWrite(run model.RunState, store *Store, draft model.DocumentDraft, onSnapshot SnapshotFn) (model.RunState, error) {
	draft = normalizeDocumentDraft(draft, run.Brief, model.Proposal{}, run.CWD)
	notify := func() {
		if onSnapshot == nil {
			return
		}
		onSnapshot(run.Clone())
	}

	run.Status = model.RunStatusRunning
	run.CurrentPhase = "writing_deliverable"
	run.StopReason = model.StopReasonDiscussionEnded
	run.FailureSummary = ""
	run.WaitingSummary = "Writing latest document version"
	run.AgentStatuses[run.Manager.ID] = updateAgentState(
		run.AgentStatuses[run.Manager.ID],
		model.AgentStateRunning,
		"writing_deliverable",
		"Retrying final document write",
		e.now(),
	)
	e.appendTimeline(&run, run.CurrentRound, run.Manager.ID, "Resuming final document write")
	e.touch(&run)
	_ = store.SaveState(run)
	notify()

	run.DeliverablePath = draft.Path
	run.FinalMarkdown = strings.TrimSpace(draft.Markdown) + "\n"
	if err := writeDeliverableFile(run.DeliverablePath, run.FinalMarkdown); err != nil {
		e.markRunFailed(&run, run.Manager.ID, "deliverable_write_failed", model.StopReasonManagerFailed, "deliverable_write_failed", err)
		e.touch(&run)
		_ = store.SaveState(run)
		notify()
		return run, err
	}

	_ = store.SaveJSON("deliverable.json", draft)
	_ = store.SaveText("deliverable.md", run.FinalMarkdown)
	run.AgentStatuses[run.Manager.ID] = updateAgentState(
		run.AgentStatuses[run.Manager.ID],
		model.AgentStateDone,
		"deliverable_written",
		"Manager finalized the deliverable",
		e.now(),
	)
	e.appendTimeline(&run, run.CurrentRound, run.Manager.ID, fmt.Sprintf("Wrote deliverable to %s", run.DeliverablePath))

	status, stopReason := resumeOutcome(run, model.Proposal{Converged: draft.Converged})
	run.PendingStatus = ""
	run.PendingStopReason = ""
	run.Status = status
	run.StopReason = stopReason
	run.FinalMarkdownPath = filepath.Join(run.OutputDir, "final.md")
	run.CurrentPhase = "finalized"
	run.WaitingSummary = ""
	e.appendTimeline(&run, run.CurrentRound, run.Manager.ID, "Manager finalized the discussion")
	e.touch(&run)
	_ = store.SaveText("final.md", run.FinalMarkdown)
	_ = store.SaveState(run)
	notify()
	return run, nil
}

func resumeOutcome(run model.RunState, proposal model.Proposal) (model.RunStatus, model.StopReason) {
	if run.PendingStatus != "" || run.PendingStopReason != "" {
		status := run.PendingStatus
		if status == "" {
			status = model.RunStatusComplete
		}
		stopReason := run.PendingStopReason
		if stopReason == "" {
			stopReason = model.StopReasonDiscussionEnded
		}
		return status, stopReason
	}
	if proposal.Converged {
		return model.RunStatusConverged, model.StopReasonConverged
	}
	if run.MaxRounds > 0 && run.CurrentRound >= run.MaxRounds {
		return model.RunStatusComplete, model.StopReasonMaxRounds
	}
	return model.RunStatusComplete, model.StopReasonDiscussionEnded
}

func resolveRunStatePath(runRef, outputRoot string) (string, error) {
	runRef = strings.TrimSpace(runRef)
	if runRef == "" {
		return "", errors.New("run reference is required")
	}

	pathLike := filepath.IsAbs(runRef) || strings.Contains(runRef, string(filepath.Separator)) || strings.HasSuffix(runRef, ".json")
	candidates := []string{}
	if pathLike {
		candidates = append(candidates, runRef)
	}
	if strings.TrimSpace(outputRoot) != "" {
		candidates = append(candidates, filepath.Join(outputRoot, runRef))
	}

	for _, candidate := range candidates {
		statePath, ok := normalizeRunStatePath(candidate)
		if !ok {
			continue
		}
		absPath, err := filepath.Abs(statePath)
		if err != nil {
			continue
		}
		if _, err := os.Stat(absPath); err == nil {
			return absPath, nil
		}
	}

	if strings.TrimSpace(outputRoot) == "" && !pathLike {
		return "", fmt.Errorf("run id %q requires an output root", runRef)
	}
	if pathLike {
		return "", fmt.Errorf("run %q was not found", runRef)
	}
	return "", fmt.Errorf("run %q was not found under %s", runRef, outputRoot)
}

func normalizeRunStatePath(candidate string) (string, bool) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return "", false
	}
	if strings.HasSuffix(candidate, string(filepath.Separator)+"state.json") || filepath.Base(candidate) == "state.json" {
		return candidate, true
	}
	return filepath.Join(candidate, "state.json"), true
}

func emptyFallback(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
