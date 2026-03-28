package model

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type ProviderID string

const (
	ProviderCodex  ProviderID = "codex"
	ProviderClaude ProviderID = "claude"
	ProviderGemini ProviderID = "gemini"
)

func (p ProviderID) String() string {
	return string(p)
}

func AllProviders() []ProviderID {
	return []ProviderID{ProviderCodex, ProviderClaude, ProviderGemini}
}

type AgentRole string

const (
	RoleManager AgentRole = "manager"
	RoleExpert  AgentRole = "expert"
)

type ExpertLens string

const (
	LensArchitecture ExpertLens = "architecture"
	LensExecution    ExpertLens = "execution"
	LensRiskQA       ExpertLens = "risk_qa"
)

func DefaultLenses(count int) []ExpertLens {
	switch count {
	case 2:
		return []ExpertLens{LensArchitecture, LensExecution}
	default:
		return []ExpertLens{LensArchitecture, LensExecution, LensRiskQA}
	}
}

type AgentState string

const (
	AgentStateIdle             AgentState = "idle"
	AgentStateQueued           AgentState = "queued"
	AgentStateStarting         AgentState = "starting"
	AgentStateRunning          AgentState = "running"
	AgentStateParsing          AgentState = "parsing"
	AgentStateDone             AgentState = "done"
	AgentStateWaitingOnManager AgentState = "waiting_on_manager"
	AgentStateWaitingOnExperts AgentState = "waiting_on_experts"
	AgentStateError            AgentState = "error"
	AgentStateSkipped          AgentState = "skipped"
)

type RunStatus string

const (
	RunStatusDrafting  RunStatus = "drafting"
	RunStatusWaiting   RunStatus = "waiting"
	RunStatusRunning   RunStatus = "running"
	RunStatusConverged RunStatus = "converged"
	RunStatusFailed    RunStatus = "failed"
	RunStatusComplete  RunStatus = "complete"
)

type StopReason string

const (
	StopReasonMaxRounds        StopReason = "max_rounds"
	StopReasonConverged        StopReason = "converged"
	StopReasonProposalStable   StopReason = "proposal_stable"
	StopReasonManagerFailed    StopReason = "manager_failed"
	StopReasonExpertsFailed    StopReason = "experts_failed"
	StopReasonCancelled        StopReason = "cancelled"
	StopReasonDiscussionEnded  StopReason = "discussion_ended"
	StopReasonAwaitingUser     StopReason = "awaiting_user"
	StopReasonNotStarted       StopReason = "not_started"
	StopReasonUnavailableAgent StopReason = "unavailable_agent"
)

type Capability struct {
	Provider      ProviderID `json:"provider"`
	DisplayName   string     `json:"display_name"`
	BinaryPath    string     `json:"binary_path"`
	Available     bool       `json:"available"`
	Authenticated bool       `json:"authenticated"`
	Summary       string     `json:"summary"`
	Error         string     `json:"error,omitempty"`
}

type AgentConfig struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Role     AgentRole  `json:"role"`
	Provider ProviderID `json:"provider"`
	Lens     ExpertLens `json:"lens,omitempty"`
}

type Request struct {
	RunID      string            `json:"run_id"`
	Round      int               `json:"round"`
	Version    int               `json:"version"`
	AgentID    string            `json:"agent_id"`
	Role       AgentRole         `json:"role"`
	Lens       ExpertLens        `json:"lens,omitempty"`
	CWD        string            `json:"cwd"`
	Prompt     string            `json:"prompt"`
	JSONSchema string            `json:"json_schema"`
	OutputKind string            `json:"output_kind"`
	Timeout    time.Duration     `json:"timeout"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type Result struct {
	Provider    ProviderID        `json:"provider"`
	StartedAt   time.Time         `json:"started_at"`
	CompletedAt time.Time         `json:"completed_at"`
	Stdout      string            `json:"stdout"`
	Stderr      string            `json:"stderr"`
	ExitCode    int               `json:"exit_code"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type ProgressEvent struct {
	Timestamp time.Time         `json:"timestamp"`
	RunID     string            `json:"run_id"`
	Round     int               `json:"round"`
	AgentID   string            `json:"agent_id"`
	Role      AgentRole         `json:"role"`
	Provider  ProviderID        `json:"provider"`
	State     AgentState        `json:"state"`
	Step      string            `json:"step"`
	Summary   string            `json:"summary"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type AgentStatus struct {
	AgentID   string     `json:"agent_id"`
	Name      string     `json:"name"`
	Role      AgentRole  `json:"role"`
	Provider  ProviderID `json:"provider"`
	Lens      ExpertLens `json:"lens,omitempty"`
	State     AgentState `json:"state"`
	LastStep  string     `json:"last_step"`
	Summary   string     `json:"summary"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type TimelineEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Round     int       `json:"round"`
	AgentID   string    `json:"agent_id,omitempty"`
	Text      string    `json:"text"`
}

type ManagerTurn struct {
	Timestamp    time.Time `json:"timestamp"`
	UserMessage  string    `json:"user_message"`
	BriefSummary string    `json:"brief_summary"`
}

type Brief struct {
	ProjectTitle  string   `json:"project_title"`
	IntentSummary string   `json:"intent_summary"`
	Goals         []string `json:"goals"`
	Constraints   []string `json:"constraints"`
	ReadyToStart  bool     `json:"ready_to_start"`
	OpenQuestions []string `json:"open_questions"`
	ManagerNotes  string   `json:"manager_notes"`
}

type PlanItem struct {
	Title   string `json:"title"`
	Details string `json:"details"`
}

type Proposal struct {
	Title           string     `json:"title"`
	Context         string     `json:"context"`
	Goals           []string   `json:"goals"`
	Constraints     []string   `json:"constraints"`
	RecommendedPlan []PlanItem `json:"recommended_plan"`
	Risks           []string   `json:"risks"`
	OpenQuestions   []string   `json:"open_questions"`
	ConsensusNotes  []string   `json:"consensus_notes"`
	Converged       bool       `json:"converged"`
	ChangeSummary   string     `json:"change_summary"`
}

type ExpertReview struct {
	Lens            ExpertLens `json:"lens"`
	Summary         string     `json:"summary"`
	Strengths       []string   `json:"strengths"`
	Concerns        []string   `json:"concerns"`
	Recommendations []string   `json:"recommendations"`
	BlockingRisks   []string   `json:"blocking_risks"`
	RequiresChanges bool       `json:"requires_changes"`
}

type RoundState struct {
	Round           int            `json:"round"`
	ProposalVersion int            `json:"proposal_version"`
	Proposal        Proposal       `json:"proposal"`
	ExpertReviews   []ExpertReview `json:"expert_reviews"`
	StartedAt       time.Time      `json:"started_at"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
}

type RunState struct {
	ID                string                 `json:"id"`
	ProjectTitle      string                 `json:"project_title"`
	CWD               string                 `json:"cwd"`
	OutputDir         string                 `json:"output_dir"`
	MaxRounds         int                    `json:"max_rounds"`
	CurrentRound      int                    `json:"current_round"`
	CurrentPhase      string                 `json:"current_phase"`
	Status            RunStatus              `json:"status"`
	WaitingSummary    string                 `json:"waiting_summary"`
	StartedAt         time.Time              `json:"started_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
	Manager           AgentConfig            `json:"manager"`
	Experts           []AgentConfig          `json:"experts"`
	AgentStatuses     map[string]AgentStatus `json:"agent_statuses"`
	Timeline          []TimelineEntry        `json:"timeline"`
	Brief             Brief                  `json:"brief"`
	ManagerTurns      []ManagerTurn          `json:"manager_turns"`
	Rounds            []RoundState           `json:"rounds"`
	FinalProposal     *Proposal              `json:"final_proposal,omitempty"`
	FinalMarkdown     string                 `json:"final_markdown,omitempty"`
	FinalMarkdownPath string                 `json:"final_markdown_path,omitempty"`
	StopReason        StopReason             `json:"stop_reason"`
}

func NewRunState(id, cwd, outputDir string, maxRounds int, manager AgentConfig, experts []AgentConfig) RunState {
	now := time.Now().UTC()
	run := RunState{
		ID:             id,
		ProjectTitle:   defaultProjectTitle(cwd),
		CWD:            cwd,
		OutputDir:      outputDir,
		MaxRounds:      maxRounds,
		Status:         RunStatusDrafting,
		CurrentPhase:   "setup",
		StartedAt:      now,
		UpdatedAt:      now,
		Manager:        manager,
		Experts:        slices.Clone(experts),
		AgentStatuses:  map[string]AgentStatus{},
		Timeline:       []TimelineEntry{},
		ManagerTurns:   []ManagerTurn{},
		Rounds:         []RoundState{},
		StopReason:     StopReasonNotStarted,
		WaitingSummary: "Waiting for the manager brief",
		Brief: Brief{
			ProjectTitle:  defaultProjectTitle(cwd),
			Goals:         []string{},
			Constraints:   []string{},
			OpenQuestions: []string{},
		},
	}
	for _, agent := range run.AllAgents() {
		run.AgentStatuses[agent.ID] = AgentStatus{
			AgentID:   agent.ID,
			Name:      agent.Name,
			Role:      agent.Role,
			Provider:  agent.Provider,
			Lens:      agent.Lens,
			State:     AgentStateIdle,
			UpdatedAt: now,
		}
	}
	run.Timeline = append(run.Timeline, TimelineEntry{
		Timestamp: now,
		Text:      "Run created",
	})
	return run
}

func (r RunState) Clone() RunState {
	data, err := json.Marshal(r)
	if err != nil {
		return r
	}
	var clone RunState
	if err := json.Unmarshal(data, &clone); err != nil {
		return r
	}
	return clone
}

func (r RunState) AllAgents() []AgentConfig {
	agents := make([]AgentConfig, 0, 1+len(r.Experts))
	agents = append(agents, r.Manager)
	agents = append(agents, r.Experts...)
	return agents
}

func (r RunState) LatestProposal() *Proposal {
	if r.FinalProposal != nil {
		return r.FinalProposal
	}
	if len(r.Rounds) == 0 {
		return nil
	}
	proposal := r.Rounds[len(r.Rounds)-1].Proposal
	return &proposal
}

func (r RunState) LatestProposalVersion() int {
	if len(r.Rounds) == 0 {
		return 0
	}
	return r.Rounds[len(r.Rounds)-1].ProposalVersion
}

func (r RunState) DisplayRound() string {
	if r.CurrentRound == 0 {
		return fmt.Sprintf("Round 0 of %d", r.MaxRounds)
	}
	return fmt.Sprintf("Round %d of %d", r.CurrentRound, r.MaxRounds)
}

func ProviderDisplayName(provider ProviderID) string {
	switch provider {
	case ProviderCodex:
		return "Codex CLI"
	case ProviderClaude:
		return "Claude CLI"
	case ProviderGemini:
		return "Gemini CLI"
	default:
		return strings.Title(provider.String())
	}
}

func defaultProjectTitle(cwd string) string {
	base := filepath.Base(strings.TrimSpace(cwd))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "Untitled project"
	}
	base = strings.ReplaceAll(base, "-", " ")
	base = strings.ReplaceAll(base, "_", " ")
	return strings.Title(base)
}
