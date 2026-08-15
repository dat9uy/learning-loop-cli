// Package opencode implements the OpenCode Adapter: the Runtime-specific
// boundary that connects one selected project through one native plugin.
// The Installer owns only .opencode/plugins/learning-loop.js; the plugin
// invokes the raw renderer and appends successful Markdown to OpenCode's
// native system context.
package opencode

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/dat9uy/learning-loop-cli/internal/pathenv"
	"github.com/dat9uy/learning-loop-cli/internal/render"
)

// ValidatedVersion is the OpenCode version whose plugin contract this
// Installer validates against.
const ValidatedVersion = "1.18.18"

// Error is a stable, code-carrying failure.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Msg
}

// Installer connects or disconnects one selected project to OpenCode.
type Installer interface {
	Install(projectRoot string) ([]string, error)
	Uninstall(projectRoot string) ([]string, error)
}

type deps struct {
	lookPath        func(string) (string, error)
	executable      func() (string, error)
	opencodeVersion func() (string, error)
}

func defaultDeps() deps {
	return deps{
		lookPath:        exec.LookPath,
		executable:      os.Executable,
		opencodeVersion: runOpenCodeVersion,
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
// Harness. It resolves both learning-loop and OpenCode from the supplied PATH
// without changing the parent process environment.
func (i *installer) InstallWithPath(projectRoot, path string) ([]string, error) {
	d := i.deps
	d.lookPath = func(name string) (string, error) { return pathenv.LookPath(path, name) }
	d.opencodeVersion = func() (string, error) { return runOpenCodeVersionWithPath(path) }
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
	if warning := i.versionWarning(); warning != "" {
		messages = append(messages, warning)
	}
	content, err := loadPlugin(projectRoot)
	if err != nil {
		return nil, err
	}
	if content == nil {
		if err := writePlugin(projectRoot, currentPluginSource); err != nil {
			return nil, err
		}
	} else {
		switch *content {
		case currentPluginSource:
			// Exact current content is already installed.
		case olderPluginSource:
			if err := writePlugin(projectRoot, currentPluginSource); err != nil {
				return nil, err
			}
		default:
			return nil, unknownPlugin()
		}
	}
	return append(messages, fmt.Sprintf("connected OpenCode to %s", projectRoot)), nil
}

func (i *installer) Uninstall(projectRoot string) ([]string, error) {
	if err := render.ValidateProjectRoot(projectRoot); err != nil {
		return nil, err
	}
	content, err := loadPlugin(projectRoot)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return []string{fmt.Sprintf("OpenCode is not connected to %s", projectRoot)}, nil
	}
	if *content != currentPluginSource && *content != olderPluginSource {
		return nil, unknownPlugin()
	}
	if err := os.Remove(pluginPath(projectRoot)); err != nil {
		return nil, &Error{Code: "E204", Msg: fmt.Sprintf("removing %s: %v", pluginPath(projectRoot), err)}
	}
	return []string{fmt.Sprintf("disconnected OpenCode from %s", projectRoot)}, nil
}

func pluginPath(projectRoot string) string {
	return filepath.Join(projectRoot, PluginDir, PluginFileName)
}

func loadPlugin(projectRoot string) (*string, error) {
	path := pluginPath(projectRoot)
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

func writePlugin(projectRoot, content string) error {
	dir := filepath.Join(projectRoot, PluginDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &Error{Code: "E204", Msg: fmt.Sprintf("creating %s: %v", dir, err)}
	}
	path := pluginPath(projectRoot)
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

func unknownPlugin() error {
	return &Error{Code: "E203", Msg: "modified or unknown learning-loop plugin in .opencode/plugins/learning-loop.js; leaving it untouched"}
}

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

var semverPattern = regexp.MustCompile(`\d+\.\d+\.\d+`)

func (i *installer) versionWarning() string {
	raw, err := i.deps.opencodeVersion()
	if err != nil {
		return "warning: could not detect the OpenCode version (opencode not found on PATH)"
	}
	version := semverPattern.FindString(raw)
	if version == "" {
		return "warning: could not detect the OpenCode version from `opencode --version` output"
	}
	if version != ValidatedVersion {
		return fmt.Sprintf("warning: OpenCode %s is not the validated %s; the plugin contract may differ", version, ValidatedVersion)
	}
	return ""
}

func runOpenCodeVersion() (string, error) {
	return runOpenCodeVersionWithPath(os.Getenv("PATH"))
}

func runOpenCodeVersionWithPath(path string) (string, error) {
	resolved, err := pathenv.LookPath(path, "opencode")
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
