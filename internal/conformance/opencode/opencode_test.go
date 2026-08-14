package opencode

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
		"model": "fake-model",
		"messages": []any{
			map[string]any{"role": "system", "content": "You are opencode, an interactive CLI tool."},
			map[string]any{"role": "system", "content": ruleBody},
			map[string]any{"role": "user", "content": harness.Prompt},
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
	if len(d.Messages) != 3 {
		t.Fatalf("messages = %d, want 3", len(d.Messages))
	}
	if d.Messages[1].Role != "system" || !strings.Contains(d.Messages[1].Text, "durable concepts") {
		t.Fatalf("message 1 = %+v", d.Messages[1])
	}
	if d.Messages[2].Role != "user" || d.Messages[2].Text != harness.Prompt {
		t.Fatalf("message 2 = %+v", d.Messages[2])
	}
}

func TestDecodeRequestHandlesPartArrayContent(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"messages": []any{
			map[string]any{
				"role": "system",
				"content": []any{
					map[string]any{"type": "text", "text": "first part"},
					map[string]any{"type": "text", "text": "second part"},
				},
			},
			map[string]any{"role": "user", "content": "hi"},
		},
	})
	d, err := (Case{}).DecodeRequest(body)
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if len(d.Messages) != 2 || d.Messages[0].Text != "first partsecond part" {
		t.Fatalf("messages = %+v, want concatenated text parts", d.Messages)
	}
}

func TestDecodeRequestIgnoresNonTextParts(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"messages": []any{
			map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{"type": "tool_use", "name": "bash", "input": map[string]any{"command": "ls"}},
				},
			},
			map[string]any{"role": "user", "content": "hi"},
		},
	})
	d, err := (Case{}).DecodeRequest(body)
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if len(d.Messages) != 2 || d.Messages[0].Text != "" {
		t.Fatalf("messages = %+v, want tool parts stripped", d.Messages)
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

func TestConfigureWritesFakeProviderConfig(t *testing.T) {
	env, err := harness.Prepare(New("/nonexistent"), harness.Options{RuntimeDir: "/nonexistent"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer os.RemoveAll(env.WorkDir)

	if err := (Case{}).Configure(env); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(env.RuntimeHome, "opencode", "opencode.json"))
	if err != nil {
		t.Fatalf("opencode.json: %v", err)
	}
	config := string(data)
	if !strings.Contains(config, env.Provider.URL()+"/v1") {
		t.Fatalf("config = %q, want the fake provider baseURL", config)
	}
	if !strings.Contains(config, `"model": "fake/fake-model"`) || !strings.Contains(config, `"small_model": "fake/fake-model"`) {
		t.Fatalf("config = %q, want both models pinned to the fake provider", config)
	}
	if strings.Contains(config, "api.openai.com") {
		t.Fatalf("config = %q, want no real provider endpoint", config)
	}
}

func TestLaunchUsesPinnedBinaryWithIsolatedEnvironment(t *testing.T) {
	dir := t.TempDir()
	record := filepath.Join(dir, "args")
	script := filepath.Join(dir, "opencode")
	content := "#!/bin/sh\nprintf '%s' \"$@\" > " + record + "\nprintf 'PWD=%s\\n' \"$PWD\" >> " + record + "\nprintf 'XDG=%s\\n' \"$XDG_CONFIG_HOME\" >> " + record + "\nprintf 'AUTO=%s\\n' \"$OPENCODE_DISABLE_AUTOUPDATE\" >> " + record + "\nprintf 'PATH=%s\\n' \"$PATH\" >> " + record + "\nexit 0\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	env, err := harness.Prepare(New(script), harness.Options{RuntimeDir: dir})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
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
	got, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("recorded args: %v", err)
	}
	text := string(got)
	for _, want := range []string{"run", harness.Prompt, "--title", "learning-loop conformance", "--print-logs"} {
		if !strings.Contains(text, want) {
			t.Fatalf("launch args = %q, want them to contain %q", text, want)
		}
	}
	if !strings.Contains(text, "PWD="+env.Project) {
		t.Fatalf("launch env = %q, want PWD = %q", text, env.Project)
	}
	if !strings.Contains(text, "XDG="+env.RuntimeHome) {
		t.Fatalf("launch env = %q, want XDG_CONFIG_HOME = %q", text, env.RuntimeHome)
	}
	if !strings.Contains(text, "AUTO=1") {
		t.Fatalf("launch env = %q, want autoupdate disabled", text)
	}
	if !strings.Contains(text, "PATH="+env.BinDir) {
		t.Fatalf("launch env = %q, want the isolated PATH", text)
	}
}
