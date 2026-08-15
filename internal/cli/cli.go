// Package cli implements the learning-loop command-line interface.
package cli

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"

	"github.com/dat9uy/learning-loop-cli/internal/codex"
	conformancecodex "github.com/dat9uy/learning-loop-cli/internal/conformance/codex"
	conformanceopencode "github.com/dat9uy/learning-loop-cli/internal/conformance/opencode"
	"github.com/dat9uy/learning-loop-cli/internal/harness"
	"github.com/dat9uy/learning-loop-cli/internal/opencode"
	"github.com/dat9uy/learning-loop-cli/internal/pi"
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
  learning-loop connect pi <project-root>        connect the project to pi
  learning-loop disconnect pi <project-root>     disconnect the project from pi
  learning-loop codex-adapter                    Codex SessionStart hook adapter (reads stdin)
  learning-loop runtime-setup <codex|opencode> [<codex|opencode>]  download pinned Runtimes into the Runtime cache
  learning-loop conformance <codex|opencode> [<codex|opencode>] [--keep]  run real-Runtime conformance cases
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
		case "pi":
			return runConnectPi(args[2], stdout, stderr)
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
		case "pi":
			return runDisconnectPi(args[2], stdout, stderr)
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
		names, err := selectRuntimes(args[1:])
		if err != nil {
			usageError(stderr)
			return 2
		}
		return runRuntimeSetup(names, stdout, stderr)
	case "conformance":
		names, keep, err := parseConformanceArgs(args[1:])
		if err != nil {
			usageError(stderr)
			return 2
		}
		return runConformance(names, keep, stdout, stderr)
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

func runConnectPi(root string, stdout, stderr io.Writer) int {
	messages, err := pi.New().Install(root)
	if err != nil {
		fmt.Fprintf(stderr, "learning-loop: connect pi: %v\n", err)
		return 1
	}
	for _, m := range messages {
		fmt.Fprintln(stdout, m)
	}
	return 0
}

func runDisconnectPi(root string, stdout, stderr io.Writer) int {
	messages, err := pi.New().Uninstall(root)
	if err != nil {
		fmt.Fprintf(stderr, "learning-loop: disconnect pi: %v\n", err)
		return 1
	}
	for _, m := range messages {
		fmt.Fprintln(stdout, m)
	}
	return 0
}

type runtimeTarget struct {
	display    string
	version    string
	binaryPath func() (string, error)
	setup      func() error
	newCase    func(string) harness.Case
}

var runtimeTargets = map[string]runtimeTarget{
	"codex": {
		display:    "Codex",
		version:    runtimecache.CodexVersion,
		binaryPath: runtimecache.CodexBinaryPath,
		setup:      runtimecache.SetupCodex,
		newCase:    func(binary string) harness.Case { return conformancecodex.New(binary) },
	},
	"opencode": {
		display:    "OpenCode",
		version:    runtimecache.OpenCodeVersion,
		binaryPath: runtimecache.OpenCodeBinaryPath,
		setup:      runtimecache.SetupOpenCode,
		newCase:    func(binary string) harness.Case { return conformanceopencode.New(binary) },
	},
}

var canonicalRuntimeNames = []string{"codex", "opencode"}

type conformanceResult struct {
	name   string
	code   int
	stdout string
	stderr string
}

// selectRuntimes validates a one- or two-Runtime selection and returns it in
// canonical order. Callers can therefore accept either input order without
// making output or scheduling order depend on the command line.
func selectRuntimes(args []string) ([]string, error) {
	if len(args) < 1 || len(args) > len(canonicalRuntimeNames) {
		return nil, fmt.Errorf("want one or two Runtime names")
	}
	seen := make(map[string]bool, len(args))
	for _, name := range args {
		if _, ok := runtimeTargets[name]; !ok {
			return nil, fmt.Errorf("unknown Runtime %q", name)
		}
		if seen[name] {
			return nil, fmt.Errorf("Runtime %q was selected more than once", name)
		}
		seen[name] = true
	}
	selected := make([]string, 0, len(args))
	for _, name := range canonicalRuntimeNames {
		if seen[name] {
			selected = append(selected, name)
		}
	}
	return selected, nil
}

func parseConformanceArgs(args []string) ([]string, bool, error) {
	if len(args) == 0 {
		return nil, false, fmt.Errorf("missing Runtime name")
	}
	keep := false
	if args[len(args)-1] == "--keep" {
		keep = true
		args = args[:len(args)-1]
	}
	names, err := selectRuntimes(args)
	return names, keep, err
}

func runRuntimeSetup(names []string, stdout, stderr io.Writer) int {
	for _, name := range names {
		target := runtimeTargets[name]
		if _, err := target.binaryPath(); err == nil {
			fmt.Fprintf(stdout, "%s %s already cached at %s\n", target.display, target.version, runtimeCachePath(name, target.version))
			continue
		}
		fmt.Fprintf(stdout, "downloading %s %s into the Runtime cache...\n", target.display, target.version)
		if err := target.setup(); err != nil {
			fmt.Fprintf(stderr, "learning-loop: runtime-setup %s: %v\n", name, err)
			return 1
		}
		fmt.Fprintf(stdout, "%s %s cached at %s\n", target.display, target.version, runtimeCachePath(name, target.version))
	}
	return 0
}

func runtimeCachePath(name, version string) string {
	dir, err := runtimecache.CacheDir()
	if err != nil {
		return "<unresolved cache>"
	}
	return filepath.Join(dir, name+"-"+version)
}

// runConformance validates every selected prerequisite before it prepares a
// disposable project. Once preflight succeeds, cases run independently and
// their captured output is emitted in canonical Runtime order.
func runConformance(names []string, keep bool, stdout, stderr io.Writer) int {
	binaries := make(map[string]string, len(names))
	preflightFailed := false
	for _, name := range names {
		path, err := runtimeTargets[name].binaryPath()
		if err != nil {
			preflightFailed = true
			fmt.Fprintf(stderr, "learning-loop: conformance %s: %v\n", name, err)
			continue
		}
		binaries[name] = path
	}
	if preflightFailed {
		return 1
	}

	return runConformanceNamed(names, keep, binaries, stdout, stderr)
}

func runConformanceNamed(names []string, keep bool, binaries map[string]string, stdout, stderr io.Writer) int {
	results := make(chan conformanceResult, len(names))
	for _, name := range names {
		name := name
		go func() {
			var out, errOut bytes.Buffer
			target := runtimeTargets[name]
			code := harness.Run(target.newCase(binaries[name]), harness.Options{
				Keep:       keep,
				RuntimeDir: filepath.Dir(binaries[name]),
			}, &out, &errOut)
			results <- conformanceResult{name: name, code: code, stdout: out.String(), stderr: errOut.String()}
		}()
	}
	byName := make(map[string]conformanceResult, len(names))
	for range names {
		item := <-results
		byName[item.name] = item
	}
	status := 0
	for _, name := range names {
		item := byName[name]
		fmt.Fprint(stdout, item.stdout)
		fmt.Fprint(stderr, item.stderr)
		if item.code != 0 {
			status = 1
		}
	}
	return status
}
