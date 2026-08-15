// Package codex implements the Codex Adapter: the Runtime-specific boundary
// that connects one selected project to Codex through its native
// session-start hook contract. The Installer owns only the recognized
// learning-loop SessionStart handler in the project's .codex/hooks.json; the
// Adapter reads the native hook event from stdin, renders Instructions
// in-process, and emits Codex's native additional-context envelope.
package codex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/dat9uy/learning-loop-cli/internal/pathenv"
	"github.com/dat9uy/learning-loop-cli/internal/render"
)

const (
	// HooksDirName is the project-local Codex configuration directory.
	HooksDirName = ".codex"
	// HooksFileName is the project-local Codex hooks file.
	HooksFileName = "hooks.json"
	// ValidatedCodexVersion is the Codex version whose session-start hook
	// contract this Installer validates against.
	ValidatedCodexVersion = "0.147.0"
)

const (
	handlerCommand       = "learning-loop codex-adapter"
	handlerStatusMessage = "learning-loop: delivering project Rules"
)

// Error is a stable, code-carrying failure.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Msg
}

// Installer connects or disconnects one selected project to Codex.
type Installer interface {
	// Install connects the project rooted at projectRoot to Codex by adding
	// the recognized learning-loop SessionStart handler to the project's
	// .codex/hooks.json. It returns the lines to report to the user.
	Install(projectRoot string) ([]string, error)
	// Uninstall removes only exact recognized learning-loop content from the
	// project's .codex/hooks.json.
	Uninstall(projectRoot string) ([]string, error)
}

type deps struct {
	lookPath     func(string) (string, error)
	executable   func() (string, error)
	codexVersion func() (string, error)
}

func defaultDeps() deps {
	return deps{
		lookPath:     exec.LookPath,
		executable:   os.Executable,
		codexVersion: runCodexVersion,
	}
}

type installer struct {
	deps deps
}

// New returns an Installer.
func New() Installer {
	return &installer{deps: defaultDeps()}
}

func (i *installer) Install(projectRoot string) ([]string, error) {
	return i.install(projectRoot)
}

// InstallWithPath is the isolated Installer seam used by the concurrent Test
// Harness. It resolves both learning-loop and Codex from the supplied PATH
// without changing the parent process environment.
func (i *installer) InstallWithPath(projectRoot, path string) ([]string, error) {
	d := i.deps
	d.lookPath = func(name string) (string, error) { return pathenv.LookPath(path, name) }
	d.codexVersion = func() (string, error) { return runCodexVersionWithPath(path) }
	return (&installer{deps: d}).install(projectRoot)
}

func (i *installer) install(projectRoot string) ([]string, error) {
	if err := render.ValidateProjectRoot(projectRoot); err != nil {
		return nil, err
	}
	if err := i.verifyPath(); err != nil {
		return nil, err
	}
	var messages []string
	if w := i.codexVersionWarning(); w != "" {
		messages = append(messages, w)
	}
	doc, err := loadHooksDoc(projectRoot)
	if err != nil {
		return nil, err
	}
	changed := false
	if doc == nil {
		doc = &hooksDoc{raw: map[string]any{}}
		changed = true
	}
	idx, state := doc.findLearningLoopGroup()
	switch state {
	case groupCurrent:
	case groupOlder:
		doc.replaceGroup(idx, currentGroup())
		changed = true
	case groupModified:
		return nil, &Error{Code: "E203", Msg: "modified or unknown learning-loop content in hooks.json; leaving it untouched"}
	case groupUnrelated:
		doc.addGroup(currentGroup())
		changed = true
	}
	if changed {
		if err := writeHooksDoc(projectRoot, doc); err != nil {
			return nil, err
		}
	}
	messages = append(messages,
		fmt.Sprintf("connected Codex to %s", projectRoot),
		"hook trust is pending: approve the learning-loop hook in Codex's hook management surface (codex TUI Hooks dialog)",
	)
	return messages, nil
}

func (i *installer) Uninstall(projectRoot string) ([]string, error) {
	if err := render.ValidateProjectRoot(projectRoot); err != nil {
		return nil, err
	}
	doc, err := loadHooksDoc(projectRoot)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return []string{fmt.Sprintf("Codex is not connected to %s", projectRoot)}, nil
	}
	idx, state := doc.findLearningLoopGroup()
	switch state {
	case groupUnrelated:
		return []string{fmt.Sprintf("Codex is not connected to %s", projectRoot)}, nil
	case groupModified:
		return nil, &Error{Code: "E203", Msg: "modified or unknown learning-loop content in hooks.json; leaving it untouched"}
	case groupCurrent, groupOlder:
		doc.removeGroup(idx)
	}
	if len(doc.raw) == 0 {
		if err := os.Remove(hooksPath(projectRoot)); err != nil {
			return nil, &Error{Code: "E204", Msg: fmt.Sprintf("removing %s: %v", hooksPath(projectRoot), err)}
		}
		return []string{fmt.Sprintf("disconnected Codex from %s", projectRoot)}, nil
	}
	if err := writeHooksDoc(projectRoot, doc); err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("disconnected Codex from %s", projectRoot)}, nil
}

// verifyPath stops installation with remediation unless PATH resolves
// learning-loop to the currently running executable.
func (i *installer) verifyPath() error {
	resolved, err := i.deps.lookPath("learning-loop")
	if err != nil {
		return &Error{Code: "E201", Msg: "learning-loop is not on PATH; add its directory to PATH and retry"}
	}
	exe, err := i.deps.executable()
	if err != nil {
		return &Error{Code: "E201", Msg: fmt.Sprintf("cannot resolve the running executable: %v", err)}
	}
	r1, err1 := filepath.EvalSymlinks(resolved)
	r2, err2 := filepath.EvalSymlinks(exe)
	if err1 != nil || err2 != nil || r1 != r2 {
		return &Error{Code: "E201", Msg: fmt.Sprintf("PATH resolves learning-loop to %s, not the running executable %s; add %s to PATH and retry", resolved, exe, filepath.Dir(exe))}
	}
	return nil
}

// codexVersionWarning returns a warning line when the detected Codex version
// is not the validated contract, or empty when it matches.
func (i *installer) codexVersionWarning() string {
	raw, err := i.deps.codexVersion()
	if err != nil {
		return "warning: could not detect the Codex version (codex not found on PATH)"
	}
	v := parseCodexVersion(raw)
	if v == "" {
		return "warning: could not detect the Codex version from `codex --version` output"
	}
	if v != ValidatedCodexVersion {
		return fmt.Sprintf("warning: Codex %s is not the validated %s; the session-start contract may differ", v, ValidatedCodexVersion)
	}
	return ""
}

var semverPattern = regexp.MustCompile(`\d+\.\d+\.\d+`)

func parseCodexVersion(raw string) string {
	return semverPattern.FindString(raw)
}

func runCodexVersion() (string, error) {
	return runCodexVersionWithPath(os.Getenv("PATH"))
}

func runCodexVersionWithPath(path string) (string, error) {
	resolved, err := pathenv.LookPath(path, "codex")
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, resolved, "--version")
	cmd.Env = pathenv.WithPath(os.Environ(), path)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
