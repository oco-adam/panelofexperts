package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"panelofexperts/internal/model"
)

func detectBinary(binary string) (string, error) {
	return exec.LookPath(binary)
}

func hasAnyFile(paths ...string) bool {
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

func authFile(pathParts ...string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	parts := append([]string{home}, pathParts...)
	return filepath.Join(parts...)
}

func emitProgress(progress chan<- model.ProgressEvent, event model.ProgressEvent) {
	if progress == nil {
		return
	}
	select {
	case progress <- event:
	default:
	}
}

func executeCommand(
	ctx context.Context,
	provider model.ProviderID,
	command string,
	args []string,
	stdin string,
	workdir string,
	request model.Request,
	progress chan<- model.ProgressEvent,
	lineHandler func(string, chan<- model.ProgressEvent, model.Request),
) (model.Result, error) {
	startedAt := time.Now().UTC()
	result := model.Result{
		Provider:  provider,
		StartedAt: startedAt,
		Metadata:  map[string]string{},
	}

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = workdir
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return result, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return result, err
	}

	emitProgress(progress, model.ProgressEvent{
		Timestamp: startedAt,
		RunID:     request.RunID,
		Round:     request.Round,
		AgentID:   request.AgentID,
		Role:      request.Role,
		Provider:  provider,
		State:     model.AgentStateStarting,
		Step:      "spawn",
		Summary:   fmt.Sprintf("Starting %s", model.ProviderDisplayName(provider)),
	})

	if err := cmd.Start(); err != nil {
		return result, err
	}

	emitProgress(progress, model.ProgressEvent{
		Timestamp: time.Now().UTC(),
		RunID:     request.RunID,
		Round:     request.Round,
		AgentID:   request.AgentID,
		Role:      request.Role,
		Provider:  provider,
		State:     model.AgentStateRunning,
		Step:      "agent_run",
		Summary:   "Agent is reasoning over the workspace",
	})

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	stdoutDone := make(chan struct{})
	stderrDone := make(chan struct{})

	go streamOutput(stdoutPipe, &stdoutBuf, stdoutDone, func(line string) {
		if lineHandler != nil {
			lineHandler(line, progress, request)
			return
		}
		if strings.TrimSpace(line) == "" {
			return
		}
		emitProgress(progress, model.ProgressEvent{
			Timestamp: time.Now().UTC(),
			RunID:     request.RunID,
			Round:     request.Round,
			AgentID:   request.AgentID,
			Role:      request.Role,
			Provider:  provider,
			State:     model.AgentStateRunning,
			Step:      "stdout",
			Summary:   "Agent produced output",
		})
	})

	go streamOutput(stderrPipe, &stderrBuf, stderrDone, func(line string) {
		if summary, ok := importantStderrSummary(line); !ok {
			return
		} else {
			emitProgress(progress, model.ProgressEvent{
				Timestamp: time.Now().UTC(),
				RunID:     request.RunID,
				Round:     request.Round,
				AgentID:   request.AgentID,
				Role:      request.Role,
				Provider:  provider,
				State:     model.AgentStateRunning,
				Step:      "stderr",
				Summary:   summary,
			})
		}
	})

	waitErr := cmd.Wait()
	<-stdoutDone
	<-stderrDone

	result.CompletedAt = time.Now().UTC()
	result.Stdout = stdoutBuf.String()
	result.Stderr = stderrBuf.String()
	result.ExitCode = exitCode(waitErr)
	if waitErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return result, fmt.Errorf("%s timed out after %s", provider, request.Timeout)
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return result, fmt.Errorf("%s was cancelled", provider)
		}
		return result, fmt.Errorf("%s exited with code %d: %w", provider, result.ExitCode, waitErr)
	}
	return result, nil
}

func streamOutput(reader io.Reader, buf *bytes.Buffer, done chan<- struct{}, lineFn func(string)) {
	defer close(done)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		buf.WriteString(line)
		buf.WriteByte('\n')
		if lineFn != nil {
			lineFn(line)
		}
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 1
	}
	return exitErr.ExitCode()
}

func summarizeLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	if len(line) > 96 {
		return line[:93] + "..."
	}
	return line
}

func importantStderrSummary(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", false
	}
	lower := strings.ToLower(line)
	if strings.Contains(lower, "shell_snapshot") ||
		strings.Contains(lower, "using filekeychain fallback") ||
		strings.Contains(lower, "loaded cached credentials") ||
		strings.Contains(lower, "loading extension:") ||
		strings.Contains(lower, "registering notification handlers") ||
		strings.Contains(lower, "supports tool updates") ||
		strings.Contains(lower, "supports prompt updates") ||
		strings.Contains(lower, "mcp context refresh") ||
		strings.Contains(lower, "rmcp::transport::worker") ||
		strings.Contains(lower, "failed to warm featured plugin") ||
		strings.Contains(lower, "thread/read failed while backfilling turn items") ||
		strings.Contains(lower, "tool execution denied by policy") ||
		strings.Contains(lower, "error executing tool exit_plan_mode") ||
		strings.Contains(lower, "<div class=") ||
		strings.Contains(lower, "<html") ||
		strings.Contains(lower, "<!doctype html") {
		return "", false
	}
	if strings.Contains(lower, "error") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "denied") ||
		strings.Contains(lower, "not logged in") {
		return summarizeLine(line), true
	}
	return "", false
}

func insideGitRepo(ctx context.Context, cwd string) bool {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

func extractJSONObject(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("empty output")
	}
	if json.Valid([]byte(raw)) {
		return raw, nil
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		candidate := raw[start : end+1]
		if json.Valid([]byte(candidate)) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("unable to locate valid JSON object in output")
}

func parseJSON[T any](raw string) (T, error) {
	var zero T
	candidate, err := extractJSONObject(raw)
	if err != nil {
		return zero, err
	}
	var value T
	if err := json.Unmarshal([]byte(candidate), &value); err != nil {
		return zero, err
	}
	return value, nil
}
