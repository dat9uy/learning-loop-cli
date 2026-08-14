// Package opencode implements the OpenCode real-Runtime conformance case:
// it launches the cached pinned OpenCode executable against the shared Test
// Harness and decodes the captured chat completions request. All shared
// fixture creation, Installer invocation, timeouts, semantic assertions,
// and diagnostic handling live in the harness; only native launch, the
// test-only provider configuration, the streaming response shape, and
// request decoding are OpenCode-specific.
package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dat9uy/learning-loop-cli/internal/harness"
	opencodeadapter "github.com/dat9uy/learning-loop-cli/internal/opencode"
	"github.com/dat9uy/learning-loop-cli/internal/runtimecache"
)

// Case is the OpenCode conformance case.
type Case struct {
	// Binary is the cached pinned OpenCode executable path.
	Binary string
}

// New returns the OpenCode conformance case for the cached pinned
// executable.
func New(binary string) Case {
	return Case{Binary: binary}
}

// Name implements harness.Case.
func (Case) Name() string {
	return "opencode"
}

// PinnedRuntime implements harness.Case.
func (Case) PinnedRuntime() string {
	return "opencode " + runtimecache.OpenCodeVersion
}

// Installer implements harness.Case: the production OpenCode Installer.
func (Case) Installer() harness.Installer {
	return opencodeadapter.New()
}

// Configure implements harness.Case: it writes the test-only fake provider
// configuration into the isolated OpenCode config home and pins both the
// session model and the small model to it, so no request can escape to a
// real model provider.
func (Case) Configure(env *harness.Env) error {
	config := map[string]any{
		"provider": map[string]any{
			"fake": map[string]any{
				"npm":  "@ai-sdk/openai-compatible",
				"name": "Fake provider",
				"options": map[string]any{
					"baseURL": env.Provider.URL() + "/v1",
					"apiKey":  "dummy",
				},
				"models": map[string]any{
					"fake-model": map[string]any{"name": "Fake model"},
				},
			},
		},
		"model":       "fake/fake-model",
		"small_model": "fake/fake-model",
		"autoupdate":  false,
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Join(env.RuntimeHome, "opencode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "opencode.json"), data, 0o644)
}

// ModelRequestPath implements harness.Case: OpenCode posts model requests
// to the chat completions endpoint.
func (Case) ModelRequestPath() string {
	return "/v1/chat/completions"
}

// Completion implements harness.Case: the canned chat completions streaming
// completion the loopback fake provider serves.
func (Case) Completion() string {
	return chatCompletion
}

// Launch implements harness.Case. It launches the cached pinned OpenCode
// executable — never whichever executable appears on PATH — with an
// explicit title so no separate title-generation request is emitted, and
// returns when it exits. PWD is set to the disposable project because the
// OpenCode runtime resolves its session directory from PWD rather than the
// process working directory.
func (c Case) Launch(ctx context.Context, env *harness.Env) (harness.LaunchResult, error) {
	args := []string{
		"run",
		env.Prompt,
		"--title", "learning-loop conformance",
		"--print-logs",
	}
	cmd := exec.CommandContext(ctx, c.Binary, args...)
	cmd.Dir = env.Project
	cmd.Env = append(os.Environ(),
		"PWD="+env.Project,
		"XDG_CONFIG_HOME="+env.RuntimeHome,
		"XDG_DATA_HOME="+filepath.Join(env.WorkDir, "data"),
		"XDG_CACHE_HOME="+filepath.Join(env.WorkDir, "cache"),
		"HOME="+filepath.Join(env.WorkDir, "home"),
		"OPENCODE_DISABLE_AUTOUPDATE=1",
		"OPENCODE_DISABLE_MODELS_FETCH=1",
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

// DecodeRequest implements harness.Case: it decodes a captured chat
// completions request into the neutral message shape the shared assertions
// consume. Message content may be a plain string or an array of typed
// parts, depending on the provider SDK version.
func (Case) DecodeRequest(body []byte) (harness.DecodedRequest, error) {
	var req struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return harness.DecodedRequest{}, fmt.Errorf("decoding the chat completions request: %v", err)
	}
	var d harness.DecodedRequest
	for _, m := range req.Messages {
		var text string
		var parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(m.Content, &parts); err == nil {
			for _, p := range parts {
				if p.Type == "text" {
					text += p.Text
				}
			}
		} else {
			json.Unmarshal(m.Content, &text)
		}
		d.Messages = append(d.Messages, harness.Message{Role: m.Role, Text: text})
	}
	return d, nil
}

// chatCompletion is a valid chat completions streaming completion: one
// assistant delta, a stop chunk, and the [DONE] terminator.
const chatCompletion = `data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":0,"model":"fake-model","choices":[{"index":0,"delta":{"role":"assistant","content":"done"},"finish_reason":null}]}

data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":0,"model":"fake-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`
