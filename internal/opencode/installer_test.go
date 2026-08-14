package opencode

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dat9uy/learning-loop-cli/internal/render"
)

func testInstaller(t *testing.T, opts ...func(*deps)) *installer {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "learning-loop")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	d := defaultDeps()
	d.lookPath = func(string) (string, error) { return exe, nil }
	d.executable = func() (string, error) { return exe, nil }
	d.opencodeVersion = func() (string, error) { return "opencode 1.18.18\n", nil }
	for _, o := range opts {
		o(&d)
	}
	return &installer{deps: d}
}

func writeTestPlugin(t *testing.T, root, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(pluginPath(root)), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(pluginPath(root), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func readPlugin(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(pluginPath(root))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(data)
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var oerr *Error
	if errors.As(err, &oerr) {
		if oerr.Code != code {
			t.Fatalf("error code = %s, want %s (%v)", oerr.Code, code, oerr)
		}
		return
	}
	var rerr *render.Error
	if errors.As(err, &rerr) {
		if rerr.Code != code {
			t.Fatalf("error code = %s, want %s (%v)", rerr.Code, code, rerr)
		}
		return
	}
	t.Fatalf("error = %v, want stable code %s", err, code)
}

func TestInstallCreatesExactPluginAndPreservesUnrelatedPlugins(t *testing.T) {
	root := t.TempDir()
	unrelated := filepath.Join(root, PluginDir, "other.js")
	if err := os.MkdirAll(filepath.Dir(unrelated), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(unrelated, []byte("export const Other = async () => ({})\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	messages, err := testInstaller(t).Install(root)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got := readPlugin(t, root); got != currentPluginSource {
		t.Fatalf("plugin = %q, want exact source %q", got, currentPluginSource)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("unrelated plugin was not preserved: %v", err)
	}
	if !strings.Contains(strings.Join(messages, "\n"), "connected OpenCode to "+root) {
		t.Fatalf("messages = %q, want connection confirmation", messages)
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	root := t.TempDir()
	inst := testInstaller(t)
	if _, err := inst.Install(root); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	path := pluginPath(root)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if _, err := inst.Install(root); err != nil {
		t.Fatalf("second Install: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) || !os.SameFile(before, after) {
		t.Fatalf("second Install rewrote the exact plugin shape")
	}
}

func TestInstallUpgradesRecognizedOlderPlugin(t *testing.T) {
	root := t.TempDir()
	writeTestPlugin(t, root, olderPluginSource)
	if _, err := testInstaller(t).Install(root); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got := readPlugin(t, root); got != currentPluginSource {
		t.Fatalf("plugin was not upgraded: %q", got)
	}
}

func TestInstallModifiedOrUnknownPluginFailsUntouched(t *testing.T) {
	for _, content := range []string{
		"",
		strings.Replace(currentPluginSource, "processFailureCode = \"E208\"", "processFailureCode = \"E999\"", 1),
		"export const LearningLoop = async () => ({})\n",
	} {
		root := t.TempDir()
		writeTestPlugin(t, root, content)
		before := readPlugin(t, root)
		_, err := testInstaller(t).Install(root)
		wantCode(t, err, "E203")
		if got := readPlugin(t, root); got != before {
			t.Fatalf("unknown plugin was touched: %q", got)
		}
	}
}

func TestInstallMalformedProjectRootFails(t *testing.T) {
	_, err := testInstaller(t).Install("relative/project")
	wantCode(t, err, "E100")
}

func TestInstallPathMismatchStopsBeforeWriting(t *testing.T) {
	root := t.TempDir()
	inst := testInstaller(t, func(d *deps) {
		d.lookPath = func(string) (string, error) { return "/usr/local/bin/learning-loop", nil }
	})
	_, err := inst.Install(root)
	wantCode(t, err, "E201")
	if _, statErr := os.Stat(pluginPath(root)); !os.IsNotExist(statErr) {
		t.Fatalf("plugin created despite PATH mismatch")
	}
}

func TestInstallWarnsForDifferentOpenCodeVersion(t *testing.T) {
	root := t.TempDir()
	inst := testInstaller(t, func(d *deps) {
		d.opencodeVersion = func() (string, error) { return "opencode 1.19.0\n", nil }
	})
	messages, err := inst.Install(root)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !strings.Contains(strings.Join(messages, "\n"), "warning: OpenCode 1.19.0 is not the validated 1.18.18") {
		t.Fatalf("messages = %q, want version warning", messages)
	}
}

func TestInstallWarnsWhenOpenCodeVersionUndetectable(t *testing.T) {
	root := t.TempDir()
	inst := testInstaller(t, func(d *deps) {
		d.opencodeVersion = func() (string, error) { return "", errors.New("not found") }
	})
	messages, err := inst.Install(root)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !strings.Contains(strings.Join(messages, "\n"), "warning: could not detect the OpenCode version") {
		t.Fatalf("messages = %q, want undetectable warning", messages)
	}
}

func TestInstallHasNoWarningForValidatedOpenCodeVersion(t *testing.T) {
	root := t.TempDir()
	messages, err := testInstaller(t).Install(root)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if strings.Contains(strings.Join(messages, "\n"), "warning:") {
		t.Fatalf("messages = %q, want no warning for validated version", messages)
	}
}

func TestInstallAtomicWriteFailureLeavesRecognizedPluginUntouched(t *testing.T) {
	root := t.TempDir()
	writeTestPlugin(t, root, olderPluginSource)
	dir := filepath.Join(root, PluginDir)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	defer os.Chmod(dir, 0o755)
	before := readPlugin(t, root)
	_, err := testInstaller(t).Install(root)
	wantCode(t, err, "E204")
	if got := readPlugin(t, root); got != before {
		t.Fatalf("recognized plugin changed after failed atomic write: %q", got)
	}
}

func TestUninstallRemovesOnlyOwnedPluginAndPreservesParents(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(root, PluginDir, "other.js")
	writeTestPlugin(t, root, currentPluginSource)
	if err := os.WriteFile(other, []byte("export const Other = async () => ({})\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	messages, err := testInstaller(t).Uninstall(root)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if _, err := os.Stat(pluginPath(root)); !os.IsNotExist(err) {
		t.Fatalf("owned plugin still exists")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("unrelated plugin was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(pluginPath(root))); err != nil {
		t.Fatalf("plugin parent directory was removed: %v", err)
	}
	if !strings.Contains(strings.Join(messages, "\n"), "disconnected OpenCode from "+root) {
		t.Fatalf("messages = %q, want disconnection confirmation", messages)
	}
}

func TestUninstallRecognizedOlderPlugin(t *testing.T) {
	root := t.TempDir()
	writeTestPlugin(t, root, olderPluginSource)
	if _, err := testInstaller(t).Uninstall(root); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if _, err := os.Stat(pluginPath(root)); !os.IsNotExist(err) {
		t.Fatalf("older owned plugin still exists")
	}
}

func TestUninstallModifiedPluginFailsUntouched(t *testing.T) {
	root := t.TempDir()
	content := "export const LearningLoop = async () => ({})\n"
	writeTestPlugin(t, root, content)
	_, err := testInstaller(t).Uninstall(root)
	wantCode(t, err, "E203")
	if got := readPlugin(t, root); got != content {
		t.Fatalf("modified plugin was touched: %q", got)
	}
}

func TestUninstallNotConnectedIsIdempotent(t *testing.T) {
	root := t.TempDir()
	messages, err := testInstaller(t).Uninstall(root)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !strings.Contains(strings.Join(messages, "\n"), "OpenCode is not connected to "+root) {
		t.Fatalf("messages = %q, want not-connected report", messages)
	}
}
