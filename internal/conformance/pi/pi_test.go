package pi

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
			map[string]any{"role": "system", "content": "You are an expert coding assistant operating inside pi.\n\n" + ruleBody},
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": harness.Prompt}}},
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
	if d.Messages[0].Role != "system" || !strings.Contains(d.Messages[0].Text, "durable concepts") {
		t.Fatalf("message 0 = %+v", d.Messages[0])
	}
	if d.Messages[1].Role != "user" || d.Messages[1].Text != harness.Prompt {
		t.Fatalf("message 1 = %+v", d.Messages[1])
	}
}

func TestDecodeRequestHandlesPartArrayContent(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "first part"},
					map[string]any{"type": "text", "text": "second part"},
				},
			},
		},
	})
	d, err := (Case{}).DecodeRequest(body)
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if len(d.Messages) != 1 || d.Messages[0].Text != "first partsecond part" {
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

func TestConfigureWritesFakeProviderModels(t *testing.T) {
	env, err := harness.Prepare(New("/nonexistent"), harness.Options{RuntimeDir: "/nonexistent"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer os.RemoveAll(env.WorkDir)

	if err := (Case{}).Configure(env); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(env.RuntimeHome, ".pi", "agent", "models.json"))
	if err != nil {
		t.Fatalf("models.json: %v", err)
	}
	config := string(data)
	if !strings.Contains(config, env.Provider.URL()+"/v1") {
		t.Fatalf("config = %q, want the fake provider baseURL", config)
	}
	if !strings.Contains(config, `"api": "openai-completions"`) {
		t.Fatalf("config = %q, want the openai-completions API", config)
	}
	if !strings.Contains(config, `"apiKey": "dummy"`) {
		t.Fatalf("config = %q, want a dummy API key", config)
	}
	if !strings.Contains(config, `"id": "fake-model"`) {
		t.Fatalf("config = %q, want the fake model", config)
	}
	if strings.Contains(config, "api.openai.com") {
		t.Fatalf("config = %q, want no real provider endpoint", config)
	}
}

func TestLaunchUsesNodeWithPinnedEntryPoint(t *testing.T) {
	dir := t.TempDir()
	record := filepath.Join(dir, "args")
	node := filepath.Join(dir, "node")
	content := "#!/bin/sh\nprintf '%s' \"$@\" > " + record + "\nprintf 'HOME=%s\\n' \"$HOME\" >> " + record + "\nprintf 'OFFLINE=%s\\n' \"$PI_OFFLINE\" >> " + record + "\nprintf 'PWD=%s\\n' \"$PWD\" >> " + record + "\nprintf 'PATH=%s\\n' \"$PATH\" >> " + record + "\nexit 0\n"
	if err := os.WriteFile(node, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	entryPoint := filepath.Join(dir, "cli.js")
	env, err := harness.Prepare(New(entryPoint), harness.Options{RuntimeDir: dir})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer os.RemoveAll(env.WorkDir)

	c := New(entryPoint)
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
	for _, want := range []string{entryPoint, "-p", "--approve", "--model", "fake/fake-model", "--no-session", harness.Prompt} {
		if !strings.Contains(text, want) {
			t.Fatalf("launch args = %q, want them to contain %q", text, want)
		}
	}
	if !strings.Contains(text, "HOME="+env.RuntimeHome) {
		t.Fatalf("launch env = %q, want HOME = %q", text, env.RuntimeHome)
	}
	if !strings.Contains(text, "OFFLINE=1") {
		t.Fatalf("launch env = %q, want offline mode", text)
	}
	if !strings.Contains(text, "PWD="+env.Project) {
		t.Fatalf("launch env = %q, want PWD = %q", text, env.Project)
	}
	if !strings.Contains(text, "PATH="+env.BinDir) {
		t.Fatalf("launch env = %q, want the isolated PATH", text)
	}
}

func TestLaunchReportsNodePrerequisite(t *testing.T) {
	env, err := harness.Prepare(New("/nonexistent"), harness.Options{RuntimeDir: "/nonexistent"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer os.RemoveAll(env.WorkDir)

	// An empty PATH cannot resolve node.
	env.InheritedPath = ""
	_, err = (Case{}).Launch(context.Background(), env)
	if err == nil || !strings.Contains(err.Error(), "node is required") {
		t.Fatalf("Launch error = %v, want the Node.js prerequisite message", err)
	}
}
