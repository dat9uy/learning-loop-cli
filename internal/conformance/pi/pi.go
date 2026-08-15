// Package pi implements the pi real-Runtime conformance case: it launches
// the cached pinned pi npm tree via node against the shared Test Harness
// and decodes the captured chat completions request. All shared fixture
// creation, Installer invocation, timeouts, semantic assertions, and
// diagnostic handling live in the harness; only native launch, the
// test-only fake provider configuration, the streaming response shape, and
// request decoding are pi-specific.
package pi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dat9uy/learning-loop-cli/internal/harness"
	"github.com/dat9uy/learning-loop-cli/internal/pathenv"
	piadapter "github.com/dat9uy/learning-loop-cli/internal/pi"
	"github.com/dat9uy/learning-loop-cli/internal/runtimecache"
)

// Case is the pi conformance case.
type Case struct {
	// EntryPoint is the cached pinned pi CLI entry point (dist/cli.js).
	EntryPoint string
}

// New returns the pi conformance case for the cached pinned entry point.
func New(entryPoint string) Case {
	return Case{EntryPoint: entryPoint}
}

// Name implements harness.Case.
func (Case) Name() string {
	return "pi"
}

// PinnedRuntime implements harness.Case.
func (Case) PinnedRuntime() string {
	return "pi " + runtimecache.PiVersion
}

// Installer implements harness.Case: the production pi Installer.
func (Case) Installer() harness.Installer {
	return piadapter.New()
}

// Configure implements harness.Case: it writes the test-only fake provider
// declaration into the isolated pi agent dir (~/.pi/agent/models.json) and
// pins the session model to it, so no request can escape to a real model
// provider.
func (Case) Configure(env *harness.Env) error {
	models := map[string]any{
		"providers": map[string]any{
			"fake": map[string]any{
				"baseUrl": env.Provider.URL() + "/v1",
				"api":     "openai-completions",
				"apiKey":  "dummy",
				"models": []any{
					map[string]any{"id": "fake-model"},
				},
			},
		},
	}
	data, err := json.MarshalIndent(models, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Join(env.RuntimeHome, ".pi", "agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "models.json"), data, 0o644)
}

// ModelRequestPath implements harness.Case: pi posts model requests to the
// chat completions endpoint.
func (Case) ModelRequestPath() string {
	return "/v1/chat/completions"
}

// Completion implements harness.Case: the canned chat completions streaming
// completion the loopback fake provider serves.
func (Case) Completion() string {
	return chatCompletion
}

// Launch implements harness.Case. It launches the cached pinned pi npm tree
// via node — never whichever pi executable appears on PATH — with the
// disposable project-trust bypass, and returns when it exits. Node.js is a
// documented prerequisite for pi conformance cases.
func (c Case) Launch(ctx context.Context, env *harness.Env) (harness.LaunchResult, error) {
	node, err := pathenv.LookPath(env.Path(), "node")
	if err != nil {
		return harness.LaunchResult{}, fmt.Errorf("node is required to run the pi conformance case; install Node.js and retry")
	}
	args := []string{
		c.EntryPoint,
		"-p",
		"--approve",
		"--model", "fake/fake-model",
		"--no-session",
		env.Prompt,
	}
	cmd := exec.CommandContext(ctx, node, args...)
	cmd.Dir = env.Project
	cmd.Env = append(os.Environ(),
		"PWD="+env.Project,
		"HOME="+env.RuntimeHome,
		"PI_OFFLINE=1",
		"PATH="+env.Path(),
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
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
// parts, depending on the message kind.
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
