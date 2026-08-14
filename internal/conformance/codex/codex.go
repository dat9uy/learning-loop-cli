// Package codex implements the Codex real-Runtime conformance case: it
// launches the cached pinned Codex executable against the shared Test
// Harness and decodes the captured Responses API request. All shared
// fixture creation, Installer invocation, timeouts, semantic assertions,
// and diagnostic handling live in the harness.
package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

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

// Launch implements harness.Case. It launches the cached pinned Codex
// executable — never whichever executable appears on PATH — with the
// disposable invocation's hook-trust bypass, and returns when it exits.
func (c Case) Launch(ctx context.Context, env *harness.Env) (harness.LaunchResult, error) {
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
		"CODEX_SQLITE_HOME="+env.SQLiteHome,
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
