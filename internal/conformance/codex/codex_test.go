package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dat9uy/learning-loop-cli/internal/harness"
)

func realisticRequest(ruleBody string) []byte {
	body, _ := json.Marshal(map[string]any{
		"model": "gpt-5",
		"input": []any{
			map[string]any{
				"type": "message",
				"role": "developer",
				"content": []any{
					map[string]any{"type": "input_text", "text": ruleBody},
				},
			},
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "Reply with the single word done."},
				},
			},
		},
	})
	return body
}

func TestDecodeRequestExtractsMessages(t *testing.T) {
	body := realisticRequest("Proposals that add or change durable concepts must:")
	d, err := (Case{}).DecodeRequest(body)
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if len(d.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(d.Messages))
	}
	if d.Messages[0].Role != "developer" || !strings.Contains(d.Messages[0].Text, "durable concepts") {
		t.Fatalf("message 0 = %+v", d.Messages[0])
	}
	if d.Messages[1].Role != "user" || d.Messages[1].Text != "Reply with the single word done." {
		t.Fatalf("message 1 = %+v", d.Messages[1])
	}
}

func TestDecodeRequestIgnoresNonMessageItems(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"input": []any{
			map[string]any{"type": "function_call", "name": "shell"},
			map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "hi"}}},
		},
	})
	d, err := (Case{}).DecodeRequest(body)
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if len(d.Messages) != 1 || d.Messages[0].Text != "hi" {
		t.Fatalf("messages = %+v, want only the user message", d.Messages)
	}
}

func TestDecodeRequestRejectsInvalidJSON(t *testing.T) {
	if _, err := (Case{}).DecodeRequest([]byte("not json")); err == nil {
		t.Fatalf("DecodeRequest accepted invalid JSON")
	}
}

func TestDecodedRequestPassesSharedAssertions(t *testing.T) {
	ruleBody := "Proposals that add or change durable concepts must:\n\n- name the preserved invariant,\n"
	d, err := (Case{}).DecodeRequest(realisticRequest(ruleBody))
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if err := harness.AssertFirstRequest(d, ruleBody, harness.Prompt); err != nil {
		t.Fatalf("AssertFirstRequest: %v", err)
	}
}

func TestConfigureMarksDisposableProjectTrusted(t *testing.T) {
	env, err := harness.Prepare(New("/nonexistent"), harness.Options{RuntimeDir: "/nonexistent"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer os.RemoveAll(env.WorkDir)

	if err := (Case{}).Configure(env); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	config, err := os.ReadFile(filepath.Join(env.RuntimeHome, "config.toml"))
	if err != nil {
		t.Fatalf("config.toml: %v", err)
	}
	if !strings.Contains(string(config), "trust_level = \"trusted\"") {
		t.Fatalf("config.toml = %q, want the disposable project marked trusted", config)
	}
}

func TestLaunchUsesPinnedBinaryWithIsolatedEnvironment(t *testing.T) {
	dir := t.TempDir()
	record := filepath.Join(dir, "args")
	script := filepath.Join(dir, "codex")
	content := "#!/bin/sh\nprintf '%s' \"$@\" > " + record + "\nexit 0\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	env, err := harness.Prepare(New(script), harness.Options{RuntimeDir: dir})
	if err != nil {
		t.Fatalf("PrepareForTest: %v", err)
	}
	defer os.RemoveAll(env.WorkDir)

	c := New(script)
	result, err := c.Launch(context.Background(), env)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	args, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("recorded args: %v", err)
	}
	got := string(args)
	for _, want := range []string{"exec", "openai_base_url=" + env.Provider.URL() + "/v1", "--dangerously-bypass-hook-trust", harness.Prompt} {
		if !strings.Contains(got, want) {
			t.Fatalf("launch args = %q, want them to contain %q", got, want)
		}
	}
}
