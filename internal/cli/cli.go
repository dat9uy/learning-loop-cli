// Package cli implements the learning-loop command-line interface.
package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/dat9uy/learning-loop-cli/internal/codex"
	conformancecodex "github.com/dat9uy/learning-loop-cli/internal/conformance/codex"
	conformanceopencode "github.com/dat9uy/learning-loop-cli/internal/conformance/opencode"
	"github.com/dat9uy/learning-loop-cli/internal/harness"
	"github.com/dat9uy/learning-loop-cli/internal/opencode"
	"github.com/dat9uy/learning-loop-cli/internal/recordstore"
	"github.com/dat9uy/learning-loop-cli/internal/render"
	"github.com/dat9uy/learning-loop-cli/internal/runtimecache"
)

const usage = `learning-loop — deliver standalone Rules as Instructions

Usage:
  learning-loop init <project-root>              initialize the project's Record Store
  learning-loop render <project-root>            render current Rules as Instruction Markdown
  learning-loop connect codex <project-root>     connect the project to Codex
  learning-loop disconnect codex <project-root> disconnect the project from Codex
  learning-loop connect opencode <project-root>  connect the project to OpenCode
  learning-loop disconnect opencode <project-root> disconnect the project from OpenCode
  learning-loop codex-adapter                    Codex SessionStart hook adapter (reads stdin)
  learning-loop runtime-setup <codex|opencode>   download the pinned Runtime into the Runtime cache
  learning-loop conformance <codex|opencode> [--keep]  run the real-Runtime conformance case
`

// Run executes the CLI and returns the process exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usageError(stderr)
		return 2
	}
	switch args[0] {
	case "init":
		if len(args) != 2 {
			usageError(stderr)
			return 2
		}
		return runInit(args[1], stdout, stderr)
	case "render":
		if len(args) != 2 {
			usageError(stderr)
			return 2
		}
		return runRender(args[1], stdout, stderr)
	case "connect":
		if len(args) != 3 {
			usageError(stderr)
			return 2
		}
		switch args[1] {
		case "codex":
			return runConnectCodex(args[2], stdout, stderr)
		case "opencode":
			return runConnectOpenCode(args[2], stdout, stderr)
		default:
			usageError(stderr)
			return 2
		}
	case "disconnect":
		if len(args) != 3 {
			usageError(stderr)
			return 2
		}
		switch args[1] {
		case "codex":
			return runDisconnectCodex(args[2], stdout, stderr)
		case "opencode":
			return runDisconnectOpenCode(args[2], stdout, stderr)
		default:
			usageError(stderr)
			return 2
		}
	case "codex-adapter":
		if len(args) != 1 {
			usageError(stderr)
			return 2
		}
		return codex.RunAdapter(stdin, stdout, stderr)
	case "runtime-setup":
		if len(args) != 2 {
			usageError(stderr)
			return 2
		}
		switch args[1] {
		case "codex":
			return runRuntimeSetupCodex(stdout, stderr)
		case "opencode":
			return runRuntimeSetupOpenCode(stdout, stderr)
		default:
			usageError(stderr)
			return 2
		}
	case "conformance":
		if len(args) < 2 {
			usageError(stderr)
			return 2
		}
		keep := false
		for _, a := range args[2:] {
			if a != "--keep" {
				usageError(stderr)
				return 2
			}
			keep = true
		}
		switch args[1] {
		case "codex":
			return runConformanceCodex(keep, stdout, stderr)
		case "opencode":
			return runConformanceOpenCode(keep, stdout, stderr)
		default:
			usageError(stderr)
			return 2
		}
	default:
		usageError(stderr)
		return 2
	}
}

func usageError(stderr io.Writer) {
	fmt.Fprint(stderr, usage)
}

func runInit(root string, stdout, stderr io.Writer) int {
	if err := render.ValidateProjectRoot(root); err != nil {
		fmt.Fprintf(stderr, "learning-loop: init: %v\n", err)
		return 1
	}
	store := recordstore.New(root)
	if err := store.Init(); err != nil {
		fmt.Fprintf(stderr, "learning-loop: init: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "initialized Record Store at %s\n", filepath.Join(root, recordstore.DirName, recordstore.RulesDirName))
	return 0
}

func runRender(root string, stdout, stderr io.Writer) int {
	out, err := render.New().Render(root)
	if err != nil {
		fmt.Fprintf(stderr, "learning-loop: render: %v\n", err)
		return 1
	}
	if _, err := stdout.Write(out); err != nil {
		fmt.Fprintf(stderr, "learning-loop: render: %v\n", err)
		return 1
	}
	return 0
}

func runConnectCodex(root string, stdout, stderr io.Writer) int {
	messages, err := codex.New().Install(root)
	if err != nil {
		fmt.Fprintf(stderr, "learning-loop: connect codex: %v\n", err)
		return 1
	}
	for _, m := range messages {
		fmt.Fprintln(stdout, m)
	}
	return 0
}

func runDisconnectCodex(root string, stdout, stderr io.Writer) int {
	messages, err := codex.New().Uninstall(root)
	if err != nil {
		fmt.Fprintf(stderr, "learning-loop: disconnect codex: %v\n", err)
		return 1
	}
	for _, m := range messages {
		fmt.Fprintln(stdout, m)
	}
	return 0
}

func runConnectOpenCode(root string, stdout, stderr io.Writer) int {
	messages, err := opencode.New().Install(root)
	if err != nil {
		fmt.Fprintf(stderr, "learning-loop: connect opencode: %v\n", err)
		return 1
	}
	for _, m := range messages {
		fmt.Fprintln(stdout, m)
	}
	return 0
}

func runDisconnectOpenCode(root string, stdout, stderr io.Writer) int {
	messages, err := opencode.New().Uninstall(root)
	if err != nil {
		fmt.Fprintf(stderr, "learning-loop: disconnect opencode: %v\n", err)
		return 1
	}
	for _, m := range messages {
		fmt.Fprintln(stdout, m)
	}
	return 0
}

func runRuntimeSetupCodex(stdout, stderr io.Writer) int {
	return runRuntimeSetup("codex", "Codex", runtimecache.CodexVersion, runtimecache.CodexBinaryPath, runtimecache.SetupCodex, stdout, stderr)
}

func runRuntimeSetupOpenCode(stdout, stderr io.Writer) int {
	return runRuntimeSetup("opencode", "OpenCode", runtimecache.OpenCodeVersion, runtimecache.OpenCodeBinaryPath, runtimecache.SetupOpenCode, stdout, stderr)
}

func runRuntimeSetup(name, display, version string, binaryPath func() (string, error), setup func() error, stdout, stderr io.Writer) int {
	if _, err := binaryPath(); err == nil {
		fmt.Fprintf(stdout, "%s %s already cached at %s\n", display, version, runtimeCachePath(name, version))
		return 0
	}
	fmt.Fprintf(stdout, "downloading %s %s into the Runtime cache...\n", display, version)
	if err := setup(); err != nil {
		fmt.Fprintf(stderr, "learning-loop: runtime-setup %s: %v\n", name, err)
		return 1
	}
	fmt.Fprintf(stdout, "%s %s cached at %s\n", display, version, runtimeCachePath(name, version))
	return 0
}

func runtimeCachePath(name, version string) string {
	dir, err := runtimecache.CacheDir()
	if err != nil {
		return "<unresolved cache>"
	}
	return filepath.Join(dir, name+"-"+version)
}

func runConformanceCodex(keep bool, stdout, stderr io.Writer) int {
	return runConformance("codex", runtimecache.CodexBinaryPath, conformancecodex.New, keep, stdout, stderr)
}

func runConformanceOpenCode(keep bool, stdout, stderr io.Writer) int {
	return runConformance("opencode", runtimecache.OpenCodeBinaryPath, conformanceopencode.New, keep, stdout, stderr)
}

func runConformance[T harness.Case](name string, binaryPath func() (string, error), newCase func(string) T, keep bool, stdout, stderr io.Writer) int {
	bin, err := binaryPath()
	if err != nil {
		fmt.Fprintf(stderr, "learning-loop: conformance %s: %v\n", name, err)
		return 1
	}
	c := newCase(bin)
	return harness.Run(c, harness.Options{Keep: keep, RuntimeDir: filepath.Dir(bin)}, stdout, stderr)
}
