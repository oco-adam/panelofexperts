package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"panelofexperts/internal/appenv"
	"panelofexperts/internal/buildinfo"
	"panelofexperts/internal/doctor"
	"panelofexperts/internal/orchestrator"
	"panelofexperts/internal/providers"
	"panelofexperts/internal/ui"
)

type App struct {
	Engine     *orchestrator.Engine
	Stdout     io.Writer
	Stderr     io.Writer
	Getwd      func() (string, error)
	Getenv     func(string) string
	HTTPClient *http.Client
	LaunchUI   func(*orchestrator.Engine, string, string, bool) error
}

func NewDefault() *App {
	return &App{
		Engine:     defaultEngine(),
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Getwd:      os.Getwd,
		Getenv:     os.Getenv,
		HTTPClient: &http.Client{Timeout: 2 * time.Second},
		LaunchUI:   ui.Run,
	}
}

func (a *App) Run(ctx context.Context, args []string) int {
	if a.Stdout == nil {
		a.Stdout = io.Discard
	}
	if a.Stderr == nil {
		a.Stderr = io.Discard
	}
	if a.Getwd == nil {
		a.Getwd = os.Getwd
	}
	if a.Getenv == nil {
		a.Getenv = os.Getenv
	}
	if a.Engine == nil {
		a.Engine = defaultEngine()
	}
	if a.LaunchUI == nil {
		a.LaunchUI = ui.Run
	}

	if len(args) > 0 {
		switch args[0] {
		case "help", "-h", "--help":
			a.printUsage()
			return 0
		case "version":
			a.printVersion()
			return 0
		case "doctor":
			return a.runDoctor(ctx, args[1:])
		}
		if !strings.HasPrefix(args[0], "-") {
			fmt.Fprintf(a.Stderr, "poe: unknown command %q\n\n", args[0])
			a.printUsage()
			return 2
		}
	}
	return a.runInteractive(args)
}

func (a *App) runInteractive(args []string) int {
	cwd, err := a.Getwd()
	if err != nil {
		fmt.Fprintf(a.Stderr, "poe: resolve cwd: %v\n", err)
		return 1
	}

	flags := flag.NewFlagSet("poe", flag.ContinueOnError)
	flags.SetOutput(a.Stderr)

	defaultOutputRoot := appenv.WorkspaceOutputRoot(cwd)
	var outputRoot string
	var debug bool
	flags.StringVar(&cwd, "cwd", cwd, "workspace directory for the discussion")
	flags.StringVar(&outputRoot, "output-root", defaultOutputRoot, "output root for run artifacts")
	flags.StringVar(&outputRoot, "output-dir", defaultOutputRoot, "output root for run artifacts")
	flags.BoolVar(&debug, "debug", false, "enable Bubble Tea debug logging")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(a.Stderr, "poe: unexpected arguments: %s\n", strings.Join(flags.Args(), " "))
		return 2
	}

	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		fmt.Fprintf(a.Stderr, "poe: resolve cwd: %v\n", err)
		return 1
	}
	if outputRoot == "" {
		outputRoot = appenv.WorkspaceOutputRoot(absCWD)
	}
	absOutputRoot, err := filepath.Abs(outputRoot)
	if err != nil {
		fmt.Fprintf(a.Stderr, "poe: resolve output root: %v\n", err)
		return 1
	}
	if err := a.LaunchUI(a.Engine, absCWD, absOutputRoot, debug); err != nil {
		fmt.Fprintf(a.Stderr, "poe: %v\n", err)
		return 1
	}
	return 0
}

func (a *App) runDoctor(ctx context.Context, args []string) int {
	cwd, err := a.Getwd()
	if err != nil {
		fmt.Fprintf(a.Stderr, "poe: resolve cwd: %v\n", err)
		return 1
	}

	flags := flag.NewFlagSet("poe doctor", flag.ContinueOnError)
	flags.SetOutput(a.Stderr)

	defaultOutputRoot := appenv.WorkspaceOutputRoot(cwd)
	var outputRoot string
	flags.StringVar(&cwd, "cwd", cwd, "workspace directory used to compute the output root")
	flags.StringVar(&outputRoot, "output-root", defaultOutputRoot, "output root to report")
	flags.StringVar(&outputRoot, "output-dir", defaultOutputRoot, "output root to report")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(a.Stderr, "poe doctor: unexpected arguments: %s\n", strings.Join(flags.Args(), " "))
		return 2
	}

	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		fmt.Fprintf(a.Stderr, "poe: resolve cwd: %v\n", err)
		return 1
	}
	if outputRoot == "" {
		outputRoot = appenv.WorkspaceOutputRoot(absCWD)
	}
	absOutputRoot, err := filepath.Abs(outputRoot)
	if err != nil {
		fmt.Fprintf(a.Stderr, "poe: resolve output root: %v\n", err)
		return 1
	}

	report, err := doctor.Gather(ctx, a.Engine, doctor.Options{
		CWD:        absCWD,
		OutputRoot: absOutputRoot,
		Getenv:     a.Getenv,
		HTTPClient: a.HTTPClient,
	})
	if err != nil {
		fmt.Fprintf(a.Stderr, "poe doctor: %v\n", err)
		return 1
	}
	fmt.Fprint(a.Stdout, doctor.RenderText(report))
	return 0
}

func (a *App) printVersion() {
	info := buildinfo.Current()
	fmt.Fprintf(a.Stdout, "%s\n", info.Summary())
	fmt.Fprintf(a.Stdout, "commit: %s\n", info.Commit)
	fmt.Fprintf(a.Stdout, "date: %s\n", info.Date)
	fmt.Fprintf(a.Stdout, "built by: %s\n", info.BuiltBy)
}

func (a *App) printUsage() {
	fmt.Fprintln(a.Stdout, "Usage:")
	fmt.Fprintln(a.Stdout, "  poe [--cwd path] [--output-root path] [--debug]")
	fmt.Fprintln(a.Stdout, "  poe version")
	fmt.Fprintln(a.Stdout, "  poe doctor [--cwd path] [--output-root path]")
}

func defaultEngine() *orchestrator.Engine {
	return orchestrator.NewEngine(
		providers.NewCodexProvider(""),
		providers.NewClaudeProvider(""),
		providers.NewGeminiProvider(""),
	)
}
