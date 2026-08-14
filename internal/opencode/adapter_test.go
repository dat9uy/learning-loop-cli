package opencode

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type pluginRunResult struct {
	System []string `json:"system"`
	Logs   []struct {
		Body struct {
			Message string `json:"message"`
		} `json:"body"`
	} `json:"logs"`
	Args    []string `json:"-"`
	Project string   `json:"-"`
}

func runPlugin(t *testing.T, mode string, loggerFails bool) pluginRunResult {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node is required to execute the native plugin: %v", err)
	}
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	argsPath := filepath.Join(root, "args")
	renderer := filepath.Join(binDir, "learning-loop")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$LEARNING_LOOP_ARGS"
case "$LEARNING_LOOP_MODE" in
success)
  printf 'rendered Instruction\n'
  ;;
render)
  printf 'learning-loop: render: E103: malformed revision\n' >&2
  exit 1
  ;;
process)
  printf 'launcher exploded\n' >&2
  exit 1
  ;;
esac
`
	if err := os.WriteFile(renderer, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	plugin := filepath.Join(root, "plugin.mjs")
	if err := os.WriteFile(plugin, []byte(currentPluginSource), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	harness := filepath.Join(root, "harness.mjs")
	const harnessSource = `import { LearningLoop } from "./plugin.mjs"

const logs = []
const hooks = await LearningLoop({
  worktree: process.env.LEARNING_LOOP_PROJECT,
  client: {
    app: {
      log: async (value) => {
        if (process.env.LEARNING_LOOP_LOG_FAIL === "1") throw new Error("logger unavailable")
        logs.push(value)
      },
    },
  },
})
const output = { system: [] }
await hooks["experimental.chat.system.transform"]({}, output)
process.stdout.write(JSON.stringify({ system: output.system, logs }))
`
	if err := os.WriteFile(harness, []byte(harnessSource), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	pathEnv := binDir + string(os.PathListSeparator) + os.Getenv("PATH")
	cmd := exec.Command("node", harness)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+pathEnv,
		"LEARNING_LOOP_ARGS="+argsPath,
		"LEARNING_LOOP_MODE="+mode,
		"LEARNING_LOOP_PROJECT="+root,
		"LEARNING_LOOP_LOG_FAIL="+map[bool]string{true: "1", false: "0"}[loggerFails],
	)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("node exit code %d: %s", exitErr.ExitCode(), exitErr.Stderr)
		}
		t.Fatalf("run node: %v", err)
	}
	var result pluginRunResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode plugin result %q: %v", output, err)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("ReadFile renderer args: %v", err)
	}
	result.Args = strings.Split(strings.TrimSuffix(string(args), "\n"), "\n")
	result.Project = root
	return result
}

func TestPluginAppendsSuccessfulRawRendererOutputAndUsesSelectedProject(t *testing.T) {
	result := runPlugin(t, "success", false)
	if len(result.System) != 1 || result.System[0] != "rendered Instruction\n" {
		t.Fatalf("system context = %#v, want rendered Instruction", result.System)
	}
	if len(result.Logs) != 0 {
		t.Fatalf("logs = %#v, want none on success", result.Logs)
	}
	if len(result.Args) != 2 || result.Args[0] != "render" || result.Args[1] != result.Project {
		t.Fatalf("renderer args = %#v, want render plus selected project", result.Args)
	}
}

func TestPluginReportsRendererFailureWithoutModelContext(t *testing.T) {
	result := runPlugin(t, "render", false)
	if len(result.System) != 0 {
		t.Fatalf("system context = %#v, want empty on renderer failure", result.System)
	}
	if len(result.Logs) != 1 || !strings.Contains(result.Logs[0].Body.Message, "E103") {
		t.Fatalf("logs = %#v, want native E103 diagnostic", result.Logs)
	}
}

func TestPluginReportsProcessFailureWithStableDiagnostic(t *testing.T) {
	result := runPlugin(t, "process", false)
	if len(result.System) != 0 {
		t.Fatalf("system context = %#v, want empty on process failure", result.System)
	}
	if len(result.Logs) != 1 || !strings.Contains(result.Logs[0].Body.Message, "E208") {
		t.Fatalf("logs = %#v, want native E208 diagnostic", result.Logs)
	}
}

func TestPluginFailsOpenWhenNativeDiagnosticSurfaceFails(t *testing.T) {
	result := runPlugin(t, "process", true)
	if len(result.System) != 0 {
		t.Fatalf("system context = %#v, want empty on process failure", result.System)
	}
	if len(result.Logs) != 0 {
		t.Fatalf("logs = %#v, want no propagated logger failure", result.Logs)
	}
}

func TestPluginContainsNoRuleSelectionOrLearningLoopSettings(t *testing.T) {
	for _, forbidden := range []string{"name:", "description:", "Rule", "recordstore", "settings"} {
		if strings.Contains(currentPluginSource, forbidden) {
			t.Fatalf("plugin contains %q; native callback must stay a raw-render bridge", forbidden)
		}
	}
}
