package providers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"panelofexperts/internal/model"
)

func TestCodexRunUsesExpectedArguments(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	stdinFile := filepath.Join(dir, "stdin.txt")
	script := writeScript(t, dir, "codex-stub.sh", `#!/bin/sh
printf '%s\n' "$@" > "$POE_ARGS_FILE"
cat > "$POE_STDIN_FILE"
output=""
while [ $# -gt 0 ]; do
  if [ "$1" = "--output-last-message" ]; then
    shift
    output="$1"
  fi
  shift
done
echo '{"type":"progress","message":"working"}'
printf '{"project_title":"X","intent_summary":"Y","goals":[],"constraints":[],"ready_to_start":true,"open_questions":[],"manager_notes":"Z"}' > "$output"
`)
	t.Setenv("POE_ARGS_FILE", argsFile)
	t.Setenv("POE_STDIN_FILE", stdinFile)

	provider := NewCodexProvider(script)
	progress := make(chan model.ProgressEvent, 8)
	_, err := provider.Run(context.Background(), model.Request{
		RunID:      "run-1",
		AgentID:    "manager",
		Role:       model.RoleManager,
		CWD:        dir,
		Prompt:     "hello from prompt",
		JSONSchema: "{}",
		OutputKind: "brief",
		Timeout:    time.Minute,
	}, progress)
	if err != nil {
		t.Fatalf("codex run: %v", err)
	}

	args := mustRead(t, argsFile)
	if !strings.Contains(args, "--skip-git-repo-check") || !strings.Contains(args, "--output-schema") || !strings.Contains(args, "--output-last-message") {
		t.Fatalf("expected codex args to include schema and git flags, got:\n%s", args)
	}
	if got := mustRead(t, stdinFile); !strings.Contains(got, "hello from prompt") {
		t.Fatalf("expected prompt on stdin, got %q", got)
	}
	if len(progress) == 0 {
		t.Fatal("expected progress events from codex run")
	}
}

func TestClaudeRunUsesPlanMode(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	cwdFile := filepath.Join(dir, "cwd.txt")
	script := writeScript(t, dir, "claude-stub.sh", `#!/bin/sh
printf '%s\n' "$@" > "$POE_ARGS_FILE"
pwd > "$POE_CWD_FILE"
printf '{"project_title":"X","intent_summary":"Y","goals":[],"constraints":[],"ready_to_start":true,"open_questions":[],"manager_notes":"Z"}'
`)
	t.Setenv("POE_ARGS_FILE", argsFile)
	t.Setenv("POE_CWD_FILE", cwdFile)

	provider := NewClaudeProvider(script)
	_, err := provider.Run(context.Background(), model.Request{
		RunID:      "run-1",
		AgentID:    "manager",
		Role:       model.RoleManager,
		CWD:        dir,
		Prompt:     "brief update",
		JSONSchema: "{}",
		OutputKind: "brief",
		Timeout:    time.Minute,
	}, make(chan model.ProgressEvent, 4))
	if err != nil {
		t.Fatalf("claude run: %v", err)
	}
	args := mustRead(t, argsFile)
	for _, expected := range []string{"-p", "--permission-mode", "plan", "--output-format", "json", "--json-schema", "brief update"} {
		if !strings.Contains(args, expected) {
			t.Fatalf("expected claude args to contain %q, got:\n%s", expected, args)
		}
	}
	if !strings.Contains(args, "--add-dir") {
		t.Fatalf("expected claude args to include workspace access flag, got:\n%s", args)
	}
	if strings.Contains(args, "--effort") {
		t.Fatalf("expected manager run to preserve the default Claude effort, got:\n%s", args)
	}
	if strings.TrimSpace(mustRead(t, cwdFile)) != dir {
		t.Fatalf("expected claude to run in request cwd when workspace access is allowed")
	}
}

func TestClaudeExpertRunUsesLowerEffort(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	script := writeScript(t, dir, "claude-stub.sh", `#!/bin/sh
printf '%s\n' "$@" > "$POE_ARGS_FILE"
printf '{"lens":"architecture","summary":"ok","strengths":[],"concerns":[],"recommendations":[],"blocking_risks":[],"requires_changes":false}'
`)
	t.Setenv("POE_ARGS_FILE", argsFile)

	provider := NewClaudeProvider(script)
	_, err := provider.Run(context.Background(), model.Request{
		RunID:      "run-1",
		AgentID:    "expert-1",
		Role:       model.RoleExpert,
		CWD:        dir,
		Prompt:     "review the plan",
		JSONSchema: "{}",
		OutputKind: "review",
		Timeout:    time.Minute,
	}, make(chan model.ProgressEvent, 4))
	if err != nil {
		t.Fatalf("claude expert run: %v", err)
	}
	args := mustRead(t, argsFile)
	for _, expected := range []string{"--effort", "low"} {
		if !strings.Contains(args, expected) {
			t.Fatalf("expected claude expert args to contain %q, got:\n%s", expected, args)
		}
	}
}

func TestClaudeRunDisablesToolsWithoutWorkspaceAccess(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	cwdFile := filepath.Join(dir, "cwd.txt")
	script := writeScript(t, dir, "claude-stub.sh", `#!/bin/sh
printf '%s\n' "$@" > "$POE_ARGS_FILE"
pwd > "$POE_CWD_FILE"
printf '{"project_title":"X","intent_summary":"Y","goals":[],"constraints":[],"ready_to_start":true,"open_questions":[],"manager_notes":"Z"}'
`)
	t.Setenv("POE_ARGS_FILE", argsFile)
	t.Setenv("POE_CWD_FILE", cwdFile)

	provider := NewClaudeProvider(script)
	_, err := provider.Run(context.Background(), model.Request{
		RunID:      "run-1",
		AgentID:    "manager",
		Role:       model.RoleManager,
		CWD:        dir,
		Prompt:     "brief update",
		JSONSchema: "{}",
		OutputKind: "brief",
		Timeout:    time.Minute,
		Metadata: map[string]string{
			"workspace_access": "none",
		},
	}, make(chan model.ProgressEvent, 4))
	if err != nil {
		t.Fatalf("claude run without workspace access: %v", err)
	}
	args := mustRead(t, argsFile)
	if strings.Contains(args, "--add-dir") {
		t.Fatalf("expected claude args to omit --add-dir, got:\n%s", args)
	}
	if strings.TrimSpace(mustRead(t, cwdFile)) == dir {
		t.Fatalf("expected claude to avoid the repo cwd when workspace access is disabled")
	}
}

func TestGeminiRunUsesJsonOutput(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	cwdFile := filepath.Join(dir, "cwd.txt")
	script := writeScript(t, dir, "gemini-stub.sh", `#!/bin/sh
printf '%s\n' "$@" > "$POE_ARGS_FILE"
pwd > "$POE_CWD_FILE"
printf '{"response":"{\"lens\":\"architecture\",\"summary\":\"ok\",\"strengths\":[],\"concerns\":[],\"recommendations\":[],\"blocking_risks\":[],\"requires_changes\":false}"}'
`)
	t.Setenv("POE_ARGS_FILE", argsFile)
	t.Setenv("POE_CWD_FILE", cwdFile)

	provider := NewGeminiProvider(script)
	result, err := provider.Run(context.Background(), model.Request{
		RunID:      "run-1",
		AgentID:    "expert-1",
		Role:       model.RoleExpert,
		CWD:        dir,
		Prompt:     "review the plan",
		JSONSchema: "{}",
		OutputKind: "review",
		Timeout:    time.Minute,
	}, make(chan model.ProgressEvent, 4))
	if err != nil {
		t.Fatalf("gemini run: %v", err)
	}
	args := mustRead(t, argsFile)
	for _, expected := range []string{"--model", "gemini-2.5-flash", "--output-format", "json", "--approval-mode", "plan", "--include-directories", dir} {
		if !strings.Contains(args, expected) {
			t.Fatalf("expected gemini args to contain %q, got:\n%s", expected, args)
		}
	}
	if strings.TrimSpace(mustRead(t, cwdFile)) != dir {
		t.Fatalf("expected gemini to run in request cwd when workspace access is allowed")
	}
	if !strings.Contains(result.Stdout, `"response"`) {
		t.Fatalf("expected gemini stdout to contain wrapped response, got %q", result.Stdout)
	}
}

func TestGeminiRunSkipsWorkspaceWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	cwdFile := filepath.Join(dir, "cwd.txt")
	script := writeScript(t, dir, "gemini-stub.sh", `#!/bin/sh
printf '%s\n' "$@" > "$POE_ARGS_FILE"
pwd > "$POE_CWD_FILE"
printf '{"response":"{\"ok\":true}"}'
`)
	t.Setenv("POE_ARGS_FILE", argsFile)
	t.Setenv("POE_CWD_FILE", cwdFile)

	provider := NewGeminiProvider(script)
	_, err := provider.Run(context.Background(), model.Request{
		RunID:      "run-1",
		AgentID:    "manager",
		Role:       model.RoleManager,
		CWD:        dir,
		Prompt:     "brief update",
		JSONSchema: "{}",
		OutputKind: "brief",
		Timeout:    time.Minute,
		Metadata: map[string]string{
			"workspace_access": "none",
		},
	}, make(chan model.ProgressEvent, 4))
	if err != nil {
		t.Fatalf("gemini run without workspace access: %v", err)
	}
	args := mustRead(t, argsFile)
	if strings.Contains(args, "--include-directories") {
		t.Fatalf("expected gemini args to omit workspace directories, got:\n%s", args)
	}
	if strings.TrimSpace(mustRead(t, cwdFile)) == dir {
		t.Fatalf("expected gemini to avoid the repo cwd when workspace access is disabled")
	}
}

func TestImportantStderrSummaryFiltersProviderNoise(t *testing.T) {
	for _, line := range []string{
		"2026-03-28T12:54:32.230113Z ERROR rmcp::transport::worker: worker quit with fatal: Transport closed",
		"2026-03-28T12:53:41.984201Z  WARN codex_exec: thread/read failed while backfilling turn items",
		"Error executing tool exit_plan_mode: Tool \"exit_plan_mode\" not found.",
		"Error executing tool write_file: Tool execution denied by policy. You are in Plan Mode.",
		"2026-03-28T13:10:54.239476Z  WARN codex_core::plugins::manager: failed to warm featured plugin",
		"<div class=\"data\"><div class=\"main-wrapper\" role=\"main\">",
	} {
		if summary, ok := importantStderrSummary(line); ok {
			t.Fatalf("expected noisy stderr to be filtered, got ok=%v summary=%q", ok, summary)
		}
	}

	if summary, ok := importantStderrSummary("fatal error: authentication failed"); !ok || !strings.Contains(summary, "authentication failed") {
		t.Fatalf("expected real stderr error to remain visible, got ok=%v summary=%q", ok, summary)
	}
}

func writeScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
