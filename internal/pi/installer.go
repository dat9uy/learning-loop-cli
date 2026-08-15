// Package pi implements the pi Adapter: the Runtime-specific boundary that
// connects one selected project to pi through its native project-local
// extension contract. The Installer owns only .pi/extensions/learning-loop.ts;
// the extension invokes the raw renderer and appends successful Markdown to
// pi's chained system prompt before the agent loop starts.
package pi

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dat9uy/learning-loop-cli/internal/pathenv"
	"github.com/dat9uy/learning-loop-cli/internal/render"
)

// trustPendingMessage is the exact remaining manual action when the user has
// not yet saved a pi project-trust decision covering the connected project.
const trustPendingMessage = "project trust is pending: trust the project in pi (approve the interactive trust prompt or run /trust) before the extension loads"

// Error is a stable, code-carrying failure.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Msg
}

// Installer connects or disconnects one selected project to pi.
type Installer interface {
	Install(projectRoot string) ([]string, error)
	Uninstall(projectRoot string) ([]string, error)
}

type deps struct {
	lookPath     func(string) (string, error)
	executable   func() (string, error)
	trustDecided func(string) bool
}

func defaultDeps() deps {
	return deps{
		lookPath:     exec.LookPath,
		executable:   os.Executable,
		trustDecided: trustDecided,
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
// Harness. It resolves learning-loop from the supplied PATH without changing
// the parent process environment.
func (i *installer) InstallWithPath(projectRoot, path string) ([]string, error) {
	d := i.deps
	d.lookPath = func(name string) (string, error) { return pathenv.LookPath(path, name) }
	return (&installer{deps: d}).install(projectRoot)
}

func (i *installer) install(projectRoot string) ([]string, error) {
	projectRoot = filepath.Clean(projectRoot)
	if err := render.ValidateProjectRoot(projectRoot); err != nil {
		return nil, err
	}
	if err := validateEmbeddableRoot(projectRoot); err != nil {
		return nil, err
	}
	if err := i.verifyPath(); err != nil {
		return nil, err
	}
	content, err := loadExtension(projectRoot)
	if err != nil {
		return nil, err
	}
	if content == nil {
		if err := writeExtension(projectRoot, currentExtensionSource(projectRoot)); err != nil {
			return nil, err
		}
	} else {
		switch classifyExtension(*content, projectRoot) {
		case extensionCurrent:
			// Exact current content is already installed.
		case extensionOtherProject:
			return nil, &Error{Code: "E203", Msg: fmt.Sprintf("learning-loop extension in %s points at a different project; leaving it untouched", extensionPath(projectRoot))}
		default:
			return nil, unknownExtension()
		}
	}
	messages := []string{fmt.Sprintf("connected pi to %s", projectRoot)}
	if !i.deps.trustDecided(projectRoot) {
		messages = append(messages, trustPendingMessage)
	}
	return messages, nil
}

func (i *installer) Uninstall(projectRoot string) ([]string, error) {
	projectRoot = filepath.Clean(projectRoot)
	if err := render.ValidateProjectRoot(projectRoot); err != nil {
		return nil, err
	}
	content, err := loadExtension(projectRoot)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return []string{fmt.Sprintf("pi is not connected to %s", projectRoot)}, nil
	}
	switch classifyExtension(*content, projectRoot) {
	case extensionCurrent:
		// Remove only the exact recognized content below.
	case extensionOtherProject:
		return nil, &Error{Code: "E203", Msg: fmt.Sprintf("learning-loop extension in %s points at a different project; leaving it untouched", extensionPath(projectRoot))}
	default:
		return nil, unknownExtension()
	}
	if err := os.Remove(extensionPath(projectRoot)); err != nil {
		return nil, &Error{Code: "E204", Msg: fmt.Sprintf("removing %s: %v", extensionPath(projectRoot), err)}
	}
	return []string{fmt.Sprintf("disconnected pi from %s", projectRoot)}, nil
}

// validateEmbeddableRoot rejects project roots that cannot be embedded in the
// extension source as a JavaScript string literal.
func validateEmbeddableRoot(projectRoot string) error {
	if strings.ContainsAny(projectRoot, "\"\\\n\r\t") {
		return &Error{Code: "E100", Msg: fmt.Sprintf("project root cannot be embedded in the pi extension: %q", projectRoot)}
	}
	return nil
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

// trustDecided reports whether the user has already saved a pi project-trust
// decision covering projectRoot. It reads only the user-owned trust.json and
// never writes it; any read or parse failure reports false so the Installer
// fails open toward reporting pending trust.
func trustDecided(projectRoot string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(home, ".pi", "agent", "trust.json"))
	if err != nil {
		return false
	}
	var decisions map[string]bool
	if err := json.Unmarshal(data, &decisions); err != nil {
		return false
	}
	// The closest saved decision on the project root or a parent path
	// applies, exactly like pi's own trust resolution.
	for dir := filepath.Clean(projectRoot); ; dir = filepath.Dir(dir) {
		if trusted, ok := decisions[dir]; ok {
			return trusted
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
	}
}

type extensionState int

const (
	extensionCurrent extensionState = iota
	extensionOtherProject
	extensionModified
)

// projectRootPattern extracts the embedded project root from the extension's
// marker line.
var projectRootPattern = regexp.MustCompile(`(?m)^const projectRoot = "([^"]*)";$`)

// classifyExtension recognizes exactly the current learning-loop extension
// shape for the selected project. A file matching the shape but pointing at a
// different project root is content the Installer does not own here; any
// other existing content is modified or unknown learning-loop content.
func classifyExtension(content, projectRoot string) extensionState {
	if content == currentExtensionSource(projectRoot) {
		return extensionCurrent
	}
	if m := projectRootPattern.FindStringSubmatch(content); m != nil && content == currentExtensionSource(m[1]) {
		return extensionOtherProject
	}
	return extensionModified
}

func extensionPath(projectRoot string) string {
	return filepath.Join(projectRoot, ExtensionDir, ExtensionFileName)
}

func loadExtension(projectRoot string) (*string, error) {
	path := extensionPath(projectRoot)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, &Error{Code: "E204", Msg: fmt.Sprintf("reading %s: %v", path, err)}
	}
	content := string(data)
	return &content, nil
}

func writeExtension(projectRoot, content string) error {
	dir := filepath.Join(projectRoot, ExtensionDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &Error{Code: "E204", Msg: fmt.Sprintf("creating %s: %v", dir, err)}
	}
	path := extensionPath(projectRoot)
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(dir, ".learning-loop-*")
	if err != nil {
		return &Error{Code: "E204", Msg: fmt.Sprintf("writing %s: %v", path, err)}
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return &Error{Code: "E204", Msg: fmt.Sprintf("writing %s: %v", path, err)}
	}
	if _, err := io.WriteString(tmp, content); err != nil {
		tmp.Close()
		return &Error{Code: "E204", Msg: fmt.Sprintf("writing %s: %v", path, err)}
	}
	if err := tmp.Close(); err != nil {
		return &Error{Code: "E204", Msg: fmt.Sprintf("writing %s: %v", path, err)}
	}
	if err := os.Rename(tmpName, path); err != nil {
		return &Error{Code: "E204", Msg: fmt.Sprintf("writing %s: %v", path, err)}
	}
	return nil
}

func unknownExtension() error {
	return &Error{Code: "E203", Msg: "modified or unknown learning-loop extension in .pi/extensions/learning-loop.ts; leaving it untouched"}
}
