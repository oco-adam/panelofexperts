package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"panelofexperts/internal/appenv"
	"panelofexperts/internal/orchestrator"
)

func TestRunVersionCommand(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
	}

	exitCode := app.Run(context.Background(), []string{"version"})
	if exitCode != 0 {
		t.Fatalf("expected success, got %d", exitCode)
	}
	output := stdout.String()
	for _, expected := range []string{"poe ", "commit:", "date:", "built by:"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in version output, got:\n%s", expected, output)
		}
	}
}

func TestRunDoctorSucceedsWithoutConfiguredProviders(t *testing.T) {
	tempDir := t.TempDir()
	var stdout bytes.Buffer
	app := &App{
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
		Getwd: func() (string, error) {
			return tempDir, nil
		},
		Getenv: func(key string) string {
			if key == "POE_HOME" {
				return t.TempDir()
			}
			return ""
		},
	}

	exitCode := app.Run(context.Background(), []string{"doctor"})
	if exitCode != 0 {
		t.Fatalf("expected doctor to exit 0, got %d", exitCode)
	}
	output := stdout.String()
	for _, expected := range []string{"Providers", "Codex CLI", "Claude CLI", "Gemini CLI"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in doctor output, got:\n%s", expected, output)
		}
	}
}

func TestRunInteractiveLaunchesUIWithResolvedPaths(t *testing.T) {
	tempDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	var launched bool
	var launchedCWD string
	var launchedOutputRoot string
	var launchedDebug bool

	app := &App{
		Stdout: &stdout,
		Stderr: &stderr,
		Getwd: func() (string, error) {
			return tempDir, nil
		},
		LaunchUI: func(_ *orchestrator.Engine, cwd, outputRoot string, debug bool) error {
			launched = true
			launchedCWD = cwd
			launchedOutputRoot = outputRoot
			launchedDebug = debug
			return nil
		},
	}

	exitCode := app.Run(context.Background(), []string{"--debug"})
	if exitCode != 0 {
		t.Fatalf("expected success, got %d", exitCode)
	}
	if !launched {
		t.Fatal("expected interactive launch to run")
	}
	if launchedCWD != tempDir {
		t.Fatalf("expected cwd %q, got %q", tempDir, launchedCWD)
	}
	expectedOutputRoot := appenv.WorkspaceOutputRoot(tempDir)
	if launchedOutputRoot != expectedOutputRoot {
		t.Fatalf("expected output root %q, got %q", expectedOutputRoot, launchedOutputRoot)
	}
	if !launchedDebug {
		t.Fatal("expected debug mode to be forwarded")
	}
}

func TestRunInteractiveReturnsErrorWhenGetwdFails(t *testing.T) {
	app := &App{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		Getwd: func() (string, error) {
			return "", errors.New("boom")
		},
	}
	if exitCode := app.Run(context.Background(), nil); exitCode != 1 {
		t.Fatalf("expected exit 1, got %d", exitCode)
	}
}
