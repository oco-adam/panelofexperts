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
	script := writeScript(t, dir, "claude-stub.sh", `#!/bin/sh
printf '%s\n' "$@" > "$POE_ARGS_FILE"
printf '{"project_title":"X","intent_summary":"Y","goals":[],"constraints":[],"ready_to_start":true,"open_questions":[],"manager_notes":"Z"}'
`)
	t.Setenv("POE_ARGS_FILE", argsFile)

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
	for _, expected := range []string{"-p", "--permission-mode", "plan", "--json-schema", "brief update"} {
		if !strings.Contains(args, expected) {
			t.Fatalf("expected claude args to contain %q, got:\n%s", expected, args)
		}
	}
}

func TestGeminiRunUsesJsonOutput(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	script := writeScript(t, dir, "gemini-stub.sh", `#!/bin/sh
printf '%s\n' "$@" > "$POE_ARGS_FILE"
printf '{"response":"{\"lens\":\"architecture\",\"summary\":\"ok\",\"strengths\":[],\"concerns\":[],\"recommendations\":[],\"blocking_risks\":[],\"requires_changes\":false}"}'
`)
	t.Setenv("POE_ARGS_FILE", argsFile)

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
	for _, expected := range []string{"--output-format", "json", "--include-directories", dir} {
		if !strings.Contains(args, expected) {
			t.Fatalf("expected gemini args to contain %q, got:\n%s", expected, args)
		}
	}
	if !strings.Contains(result.Stdout, `"response"`) {
		t.Fatalf("expected gemini stdout to contain wrapped response, got %q", result.Stdout)
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
