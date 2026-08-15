// Package codex implements the Codex real-Runtime conformance case: it
// launches the cached pinned Codex executable against the shared Test
// Harness and decodes the captured Responses API request. All shared
// fixture creation, Installer invocation, timeouts, semantic assertions,
// and diagnostic handling live in the harness; only native launch, the
// disposable project trust configuration, the streaming response shape, and
// request decoding are Codex-specific.
package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	codexadapter "github.com/dat9uy/learning-loop-cli/internal/codex"
	"github.com/dat9uy/learning-loop-cli/internal/harness"
	"github.com/dat9uy/learning-loop-cli/internal/runtimecache"
)

// Case is the Codex conformance case.
type Case struct {
	// Binary is the cached pinned Codex executable path.
	Binary string
}

// New returns the Codex conformance case for the cached pinned executable.
func New(binary string) Case {
	return Case{Binary: binary}
}

// Name implements harness.Case.
func (Case) Name() string {
	return "codex"
}

// PinnedRuntime implements harness.Case.
func (Case) PinnedRuntime() string {
	return "codex " + runtimecache.CodexVersion
}

// Installer implements harness.Case: the production Codex Installer.
func (Case) Installer() harness.Installer {
	return codexadapter.New()
}

// Configure implements harness.Case: it writes the disposable project trust
// into the isolated Codex home. Codex gates project-local hooks on project
// trust, and only this harness-owned disposable invocation may bypass it.
func (Case) Configure(env *harness.Env) error {
	canonical, err := filepath.EvalSymlinks(env.Project)
	if err != nil {
		return err
	}
	config := fmt.Sprintf("[projects.%q]\ntrust_level = \"trusted\"\n", canonical)
	return os.WriteFile(filepath.Join(env.RuntimeHome, "config.toml"), []byte(config), 0o644)
}

// ModelRequestPath implements harness.Case: Codex posts model requests to
// the Responses API.
func (Case) ModelRequestPath() string {
	return "/v1/responses"
}

// Completion implements harness.Case: the canned Responses API streaming
// completion the loopback fake provider serves.
func (Case) Completion() string {
	return responsesCompletion
}

// Launch implements harness.Case. It launches the cached pinned Codex
// executable — never whichever executable appears on PATH — with the
// disposable invocation's hook-trust bypass, and returns when it exits.
func (c Case) Launch(ctx context.Context, env *harness.Env) (harness.LaunchResult, error) {
	sqliteHome := filepath.Join(env.WorkDir, "sqlite-home")
	if err := os.MkdirAll(sqliteHome, 0o755); err != nil {
		return harness.LaunchResult{}, err
	}
	args := []string{
		"exec",
		"-c", "openai_base_url=" + env.Provider.URL() + "/v1",
		"--dangerously-bypass-hook-trust",
		env.Prompt,
	}
	cmd := exec.CommandContext(ctx, c.Binary, args...)
	cmd.Dir = env.Project
	cmd.Env = append(os.Environ(),
		"CODEX_HOME="+env.RuntimeHome,
		"CODEX_SQLITE_HOME="+sqliteHome,
		"HOME="+filepath.Join(env.WorkDir, "home"),
		"XDG_CONFIG_HOME="+filepath.Join(env.WorkDir, "config"),
		"XDG_DATA_HOME="+filepath.Join(env.WorkDir, "data"),
		"XDG_CACHE_HOME="+filepath.Join(env.WorkDir, "cache"),
		"OPENAI_API_KEY=dummy",
		"PATH="+env.Path(),
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := harness.LaunchResult{Args: args, Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, err
	}
	result.ExitCode = 0
	return result, nil
}

// DecodeRequest implements harness.Case: it decodes a captured Responses
// API request into the neutral message shape the shared assertions consume.
func (Case) DecodeRequest(body []byte) (harness.DecodedRequest, error) {
	var req struct {
		Input []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"input"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return harness.DecodedRequest{}, fmt.Errorf("decoding the Responses API request: %v", err)
	}
	var d harness.DecodedRequest
	for _, item := range req.Input {
		if item.Type != "message" {
			continue
		}
		var text string
		for _, c := range item.Content {
			if c.Type == "input_text" || c.Type == "output_text" {
				text += c.Text
			}
		}
		d.Messages = append(d.Messages, harness.Message{Role: item.Role, Text: text})
	}
	return d, nil
}

// responsesCompletion is a valid Responses API streaming completion: a
// created response, one assistant output item, and a completed response. It
// mirrors the event shape the pinned Codex CLI's own test suite serves.
const responsesCompletion = `event: response.created
data: {"type":"response.created","response":{"id":"response_1"}}

event: response.output_item.done
data: {"type":"response.output_item.done","item":{"type":"message","role":"assistant","id":"response_1","content":[{"type":"output_text","text":"done"}]}}

event: response.completed
data: {"type":"response.completed","response":{"id":"response_1","usage":{"input_tokens":0,"input_tokens_details":null,"output_tokens":0,"output_tokens_details":null,"total_tokens":0}}}

`
