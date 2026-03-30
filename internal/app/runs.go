package app

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"panelofexperts/internal/appenv"
	"panelofexperts/internal/model"
	"panelofexperts/internal/orchestrator"
)

func (a *App) runRuns(args []string) int {
	cwd, err := a.defaultCWD()
	if err != nil {
		fmt.Fprintf(a.Stderr, "poe runs: resolve cwd: %v\n", err)
		return 1
	}

	flags := flag.NewFlagSet("poe runs", flag.ContinueOnError)
	flags.SetOutput(a.Stderr)

	defaultOutputRoot := appenv.WorkspaceOutputRoot(cwd)
	var outputRoot string
	flags.StringVar(&cwd, "cwd", cwd, "workspace directory used to compute the output root")
	flags.StringVar(&outputRoot, "output-root", defaultOutputRoot, "output root for run artifacts")
	flags.StringVar(&outputRoot, "output-dir", defaultOutputRoot, "output root for run artifacts")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(a.Stderr, "poe runs: unexpected arguments: %s\n", strings.Join(flags.Args(), " "))
		return 2
	}
	if !flagWasSet(flags, "output-root") && !flagWasSet(flags, "output-dir") {
		outputRoot = appenv.WorkspaceOutputRoot(cwd)
	}

	_, absOutputRoot, err := resolvePaths(cwd, outputRoot)
	if err != nil {
		fmt.Fprintf(a.Stderr, "poe runs: %v\n", err)
		return 1
	}

	runs, err := orchestrator.ListRuns(absOutputRoot)
	if err != nil {
		fmt.Fprintf(a.Stderr, "poe runs: %v\n", err)
		return 1
	}
	fmt.Fprint(a.Stdout, renderRunList(absOutputRoot, runs))
	return 0
}

func (a *App) runRetry(ctx context.Context, args []string) int {
	cwd, err := a.defaultCWD()
	if err != nil {
		fmt.Fprintf(a.Stderr, "poe retry: resolve cwd: %v\n", err)
		return 1
	}

	flags := flag.NewFlagSet("poe retry", flag.ContinueOnError)
	flags.SetOutput(a.Stderr)

	defaultOutputRoot := appenv.WorkspaceOutputRoot(cwd)
	var outputRoot string
	var runRef string
	var deliverableTimeout DurationFlag
	flags.StringVar(&cwd, "cwd", cwd, "workspace directory used to compute the output root")
	flags.StringVar(&outputRoot, "output-root", defaultOutputRoot, "output root for run artifacts")
	flags.StringVar(&outputRoot, "output-dir", defaultOutputRoot, "output root for run artifacts")
	flags.StringVar(&runRef, "run", "", "run id under the output root or an absolute run path")
	flags.Var(&deliverableTimeout, "deliverable-timeout", "override timeout for the final deliverable phase, for example 90m or 2h")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(a.Stderr, "poe retry: unexpected arguments: %s\n", strings.Join(flags.Args(), " "))
		return 2
	}
	if strings.TrimSpace(runRef) == "" {
		fmt.Fprintln(a.Stderr, "poe retry: --run is required")
		return 2
	}
	if !flagWasSet(flags, "output-root") && !flagWasSet(flags, "output-dir") {
		outputRoot = appenv.WorkspaceOutputRoot(cwd)
	}

	_, absOutputRoot, err := resolvePaths(cwd, outputRoot)
	if err != nil {
		fmt.Fprintf(a.Stderr, "poe retry: %v\n", err)
		return 1
	}

	run, err := orchestrator.LoadRun(runRef, absOutputRoot)
	if err != nil {
		fmt.Fprintf(a.Stderr, "poe retry: %v\n", err)
		return 1
	}
	timeout := run.DeliverableTimeout
	if deliverableTimeout.Value > 0 {
		timeout = deliverableTimeout.Value
	}
	if timeout <= 0 {
		timeout = time.Hour
	}
	fmt.Fprintf(a.Stdout, "Retrying %s from %s with deliverable timeout %s\n", run.ID, run.CurrentPhase, timeout)

	updated, err := a.Engine.ResumeRun(ctx, run, orchestrator.ResumeRunOptions{
		DeliverableTimeout: deliverableTimeout.Value,
	}, nil)
	if err != nil {
		fmt.Fprintf(a.Stderr, "poe retry: %v\n", err)
		if strings.TrimSpace(updated.OutputDir) != "" {
			fmt.Fprint(a.Stderr, renderSingleRun("Saved run state", updated))
		}
		return 1
	}

	fmt.Fprint(a.Stdout, renderSingleRun("Retry completed", updated))
	return 0
}

func (a *App) runRerun(ctx context.Context, args []string) int {
	cwd, err := a.defaultCWD()
	if err != nil {
		fmt.Fprintf(a.Stderr, "poe rerun: resolve cwd: %v\n", err)
		return 1
	}

	flags := flag.NewFlagSet("poe rerun", flag.ContinueOnError)
	flags.SetOutput(a.Stderr)

	defaultOutputRoot := appenv.WorkspaceOutputRoot(cwd)
	var outputRoot string
	var runRef string
	var manager string
	var experts string
	var maxRounds int
	var mergeMode string
	var deliverableTimeout DurationFlag
	flags.StringVar(&cwd, "cwd", cwd, "workspace directory used to compute the output root when resolving run ids")
	flags.StringVar(&outputRoot, "output-root", defaultOutputRoot, "output root for run lookup and optional rerun output")
	flags.StringVar(&outputRoot, "output-dir", defaultOutputRoot, "output root for run lookup and optional rerun output")
	flags.StringVar(&runRef, "run", "", "run id under the output root or an absolute run path")
	flags.StringVar(&manager, "manager", "", "override manager provider: codex, claude, or gemini")
	flags.StringVar(&experts, "experts", "", "comma-separated expert providers to use for the rerun")
	flags.IntVar(&maxRounds, "max-rounds", 0, "override max discussion rounds")
	flags.StringVar(&mergeMode, "merge", "", "override merge strategy: together or sequential")
	flags.Var(&deliverableTimeout, "deliverable-timeout", "override timeout for the final deliverable phase, for example 90m or 2h")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(a.Stderr, "poe rerun: unexpected arguments: %s\n", strings.Join(flags.Args(), " "))
		return 2
	}
	if strings.TrimSpace(runRef) == "" {
		fmt.Fprintln(a.Stderr, "poe rerun: --run is required")
		return 2
	}
	outputRootWasSet := flagWasSet(flags, "output-root") || flagWasSet(flags, "output-dir")
	if !outputRootWasSet {
		outputRoot = appenv.WorkspaceOutputRoot(cwd)
	}
	_, absOutputRoot, err := resolvePaths(cwd, outputRoot)
	if err != nil {
		fmt.Fprintf(a.Stderr, "poe rerun: %v\n", err)
		return 1
	}

	source, err := orchestrator.LoadRun(runRef, absOutputRoot)
	if err != nil {
		fmt.Fprintf(a.Stderr, "poe rerun: %v\n", err)
		return 1
	}

	var managerProvider model.ProviderID
	if strings.TrimSpace(manager) != "" {
		managerProvider, err = parseProviderID(manager)
		if err != nil {
			fmt.Fprintf(a.Stderr, "poe rerun: %v\n", err)
			return 2
		}
	}

	var expertProviders []model.ProviderID
	if strings.TrimSpace(experts) != "" {
		expertProviders, err = parseProviderList(experts)
		if err != nil {
			fmt.Fprintf(a.Stderr, "poe rerun: %v\n", err)
			return 2
		}
	}

	var strategy model.MergeStrategy
	if strings.TrimSpace(mergeMode) != "" {
		strategy, err = parseMergeStrategy(mergeMode)
		if err != nil {
			fmt.Fprintf(a.Stderr, "poe rerun: %v\n", err)
			return 2
		}
	}

	options := orchestrator.RerunOptions{
		MaxRounds:          maxRounds,
		MergeStrategy:      strategy,
		DeliverableTimeout: deliverableTimeout.Value,
		ManagerProvider:    managerProvider,
		ExpertProviders:    expertProviders,
	}
	if outputRootWasSet {
		options.OutputRoot = absOutputRoot
	}

	fmt.Fprintf(a.Stdout, "Starting rerun from %s\n", source.ID)
	updated, err := a.Engine.RerunFromRun(ctx, source, options, nil)
	if err != nil {
		fmt.Fprintf(a.Stderr, "poe rerun: %v\n", err)
		if strings.TrimSpace(updated.OutputDir) != "" {
			fmt.Fprint(a.Stderr, renderSingleRun("Saved rerun state", updated))
		}
		return 1
	}

	fmt.Fprint(a.Stdout, renderSingleRun("Rerun completed", updated))
	return 0
}

type DurationFlag struct {
	Value time.Duration
}

func (d *DurationFlag) Set(value string) error {
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return err
	}
	d.Value = parsed
	return nil
}

func (d *DurationFlag) String() string {
	if d == nil || d.Value <= 0 {
		return ""
	}
	return d.Value.String()
}

func renderRunList(outputRoot string, runs []model.RunState) string {
	if len(runs) == 0 {
		return fmt.Sprintf("No runs found under %s\n", outputRoot)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Runs under %s\n\n", outputRoot)
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tPHASE\tUPDATED\tACTIONS\tPROJECT")
	for _, run := range runs {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t%s\n",
			run.ID,
			run.Status,
			run.CurrentPhase,
			run.UpdatedAt.Format("2006-01-02 15:04:05Z07:00"),
			strings.Join(runActions(run), ", "),
			run.ProjectTitle,
		)
	}
	_ = w.Flush()
	b.WriteString("\nUse `poe retry --run <id>` to resume an interrupted or failed final deliverable phase in place.\n")
	b.WriteString("Use `poe rerun --run <id>` to start a fresh run from a saved brief with optional provider, round, merge, or timeout overrides.\n")
	return b.String()
}

func renderSingleRun(header string, run model.RunState) string {
	lines := []string{
		header,
		fmt.Sprintf("Run: %s", run.ID),
		fmt.Sprintf("Workspace: %s", run.CWD),
		fmt.Sprintf("Output: %s", run.OutputDir),
		fmt.Sprintf("Status: %s", run.Status),
		fmt.Sprintf("Phase: %s", run.CurrentPhase),
	}
	if run.DeliverableTimeout > 0 {
		lines = append(lines, fmt.Sprintf("Deliverable timeout: %s", run.DeliverableTimeout))
	}
	if strings.TrimSpace(run.DeliverablePath) != "" {
		lines = append(lines, fmt.Sprintf("Deliverable: %s", run.DeliverablePath))
	}
	if strings.TrimSpace(run.FinalMarkdownPath) != "" {
		lines = append(lines, fmt.Sprintf("Final markdown: %s", run.FinalMarkdownPath))
	}
	if strings.TrimSpace(run.FailureSummary) != "" {
		lines = append(lines, fmt.Sprintf("Failure: %s", run.FailureSummary))
	}
	return strings.Join(lines, "\n") + "\n"
}

func runActions(run model.RunState) []string {
	actions := []string{"rerun"}
	if orchestrator.CanResumeRun(run) {
		actions = append([]string{"retry"}, actions...)
	}
	return actions
}

func resolvePaths(cwd, outputRoot string) (string, string, error) {
	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		return "", "", fmt.Errorf("resolve cwd: %w", err)
	}
	if strings.TrimSpace(outputRoot) == "" {
		outputRoot = appenv.WorkspaceOutputRoot(absCWD)
	}
	absOutputRoot, err := filepath.Abs(outputRoot)
	if err != nil {
		return "", "", fmt.Errorf("resolve output root: %w", err)
	}
	return absCWD, absOutputRoot, nil
}

func (a *App) defaultCWD() (string, error) {
	if a.Getwd == nil {
		a.Getwd = os.Getwd
	}
	return a.Getwd()
}

func flagWasSet(flags *flag.FlagSet, name string) bool {
	set := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

func parseProviderID(raw string) (model.ProviderID, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(model.ProviderCodex):
		return model.ProviderCodex, nil
	case string(model.ProviderClaude):
		return model.ProviderClaude, nil
	case string(model.ProviderGemini):
		return model.ProviderGemini, nil
	default:
		return "", fmt.Errorf("unknown provider %q", raw)
	}
}

func parseProviderList(raw string) ([]model.ProviderID, error) {
	parts := strings.Split(raw, ",")
	providers := make([]model.ProviderID, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		provider, err := parseProviderID(part)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	if len(providers) < 2 || len(providers) > 3 {
		return nil, fmt.Errorf("expected 2 or 3 expert providers, got %d", len(providers))
	}
	return providers, nil
}

func parseMergeStrategy(raw string) (model.MergeStrategy, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(model.MergeStrategyTogether):
		return model.MergeStrategyTogether, nil
	case string(model.MergeStrategySequential):
		return model.MergeStrategySequential, nil
	default:
		return "", fmt.Errorf("unknown merge strategy %q", raw)
	}
}
