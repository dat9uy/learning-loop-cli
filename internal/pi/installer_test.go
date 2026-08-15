package pi

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
	d.trustDecided = func(string) bool { return false }
	for _, o := range opts {
		o(&d)
	}
	return &installer{deps: d}
}

func writeTestExtension(t *testing.T, root, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(extensionPath(root)), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(extensionPath(root), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func readExtension(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(extensionPath(root))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(data)
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var perr *Error
	if errors.As(err, &perr) {
		if perr.Code != code {
			t.Fatalf("error code = %s, want %s (%v)", perr.Code, code, perr)
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

func TestInstallCreatesExactExtensionAndReportsTrust(t *testing.T) {
	root := t.TempDir()
	messages, err := testInstaller(t).Install(root)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got := readExtension(t, root); got != currentExtensionSource(root) {
		t.Fatalf("extension = %q, want exact source %q", got, currentExtensionSource(root))
	}
	joined := strings.Join(messages, "\n")
	if !strings.Contains(joined, "connected pi to "+root) {
		t.Fatalf("messages = %q, want connection confirmation", joined)
	}
	if !strings.Contains(joined, "project trust is pending") {
		t.Fatalf("messages = %q, want trust pending report", joined)
	}
	if !strings.Contains(joined, "/trust") {
		t.Fatalf("messages = %q, want the exact manual trust action", joined)
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	root := t.TempDir()
	inst := testInstaller(t)
	if _, err := inst.Install(root); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	path := extensionPath(root)
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
		t.Fatalf("second Install rewrote the exact extension shape")
	}
}

func TestInstallCleansTrailingSlash(t *testing.T) {
	root := t.TempDir()
	inst := testInstaller(t)
	if _, err := inst.Install(root + string(os.PathSeparator)); err != nil {
		t.Fatalf("Install with trailing slash: %v", err)
	}
	if _, err := inst.Install(root); err != nil {
		t.Fatalf("Install without trailing slash: %v", err)
	}
	if got := readExtension(t, root); got != currentExtensionSource(root) {
		t.Fatalf("extension = %q, want exact source for cleaned root", got)
	}
}

func TestInstallModifiedOrUnknownExtensionFailsUntouched(t *testing.T) {
	root := t.TempDir()
	for _, content := range []string{
		"",
		strings.Replace(currentExtensionSource(root), "processFailureCode = \"E209\"", "processFailureCode = \"E999\"", 1),
		"export default function (pi) {}\n",
	} {
		root := t.TempDir()
		writeTestExtension(t, root, content)
		before := readExtension(t, root)
		_, err := testInstaller(t).Install(root)
		wantCode(t, err, "E203")
		if got := readExtension(t, root); got != before {
			t.Fatalf("unknown extension was touched: %q", got)
		}
	}
}

func TestInstallExtensionForOtherProjectFailsUntouched(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(t.TempDir(), "other-project")
	writeTestExtension(t, root, currentExtensionSource(other))
	before := readExtension(t, root)
	_, err := testInstaller(t).Install(root)
	wantCode(t, err, "E203")
	if !strings.Contains(err.Error(), "different project") {
		t.Fatalf("error = %v, want different-project report", err)
	}
	if got := readExtension(t, root); got != before {
		t.Fatalf("other-project extension was touched: %q", got)
	}
}

func TestInstallPreservesUnrelatedExtensions(t *testing.T) {
	root := t.TempDir()
	unrelated := filepath.Join(root, ExtensionDir, "other.ts")
	if err := os.MkdirAll(filepath.Dir(unrelated), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(unrelated, []byte("export default function (pi) {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := testInstaller(t).Install(root); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("unrelated extension was not preserved: %v", err)
	}
}

func TestInstallSkipsTrustReportWhenAlreadyTrusted(t *testing.T) {
	root := t.TempDir()
	inst := testInstaller(t, func(d *deps) {
		d.trustDecided = func(string) bool { return true }
	})
	messages, err := inst.Install(root)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if strings.Contains(strings.Join(messages, "\n"), "project trust is pending") {
		t.Fatalf("messages = %q, want no pending report for a trusted project", messages)
	}
}

func TestInstallMalformedProjectRootFails(t *testing.T) {
	_, err := testInstaller(t).Install("relative/project")
	wantCode(t, err, "E100")
}

func TestInstallRootWithQuoteFails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "a\"b")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	_, err := testInstaller(t).Install(root)
	wantCode(t, err, "E100")
	if _, statErr := os.Stat(extensionPath(root)); !os.IsNotExist(statErr) {
		t.Fatalf("extension created despite unembeddable root")
	}
}

func TestInstallPathMismatchStopsBeforeWriting(t *testing.T) {
	root := t.TempDir()
	inst := testInstaller(t, func(d *deps) {
		d.lookPath = func(string) (string, error) { return "/usr/local/bin/learning-loop", nil }
	})
	_, err := inst.Install(root)
	wantCode(t, err, "E201")
	if _, statErr := os.Stat(extensionPath(root)); !os.IsNotExist(statErr) {
		t.Fatalf("extension created despite PATH mismatch")
	}
}

func TestInstallWriteFailureCreatesNothing(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ExtensionDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	defer os.Chmod(dir, 0o755)
	_, err := testInstaller(t).Install(root)
	wantCode(t, err, "E204")
	if _, statErr := os.Stat(extensionPath(root)); !os.IsNotExist(statErr) {
		t.Fatalf("extension created despite failed write")
	}
}

func TestUninstallRemovesOnlyOwnedExtensionAndPreservesParents(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(root, ExtensionDir, "other.ts")
	writeTestExtension(t, root, currentExtensionSource(root))
	if err := os.WriteFile(other, []byte("export default function (pi) {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	messages, err := testInstaller(t).Uninstall(root)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if _, err := os.Stat(extensionPath(root)); !os.IsNotExist(err) {
		t.Fatalf("owned extension still exists")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("unrelated extension was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(extensionPath(root))); err != nil {
		t.Fatalf("extension parent directory was removed: %v", err)
	}
	if !strings.Contains(strings.Join(messages, "\n"), "disconnected pi from "+root) {
		t.Fatalf("messages = %q, want disconnection confirmation", messages)
	}
}

func TestUninstallModifiedExtensionFailsUntouched(t *testing.T) {
	root := t.TempDir()
	content := "export default function (pi) {}\n"
	writeTestExtension(t, root, content)
	_, err := testInstaller(t).Uninstall(root)
	wantCode(t, err, "E203")
	if got := readExtension(t, root); got != content {
		t.Fatalf("modified extension was touched: %q", got)
	}
}

func TestUninstallExtensionForOtherProjectFailsUntouched(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(t.TempDir(), "other-project")
	writeTestExtension(t, root, currentExtensionSource(other))
	before := readExtension(t, root)
	_, err := testInstaller(t).Uninstall(root)
	wantCode(t, err, "E203")
	if got := readExtension(t, root); got != before {
		t.Fatalf("other-project extension was touched: %q", got)
	}
}

func TestUninstallNotConnectedIsIdempotent(t *testing.T) {
	root := t.TempDir()
	messages, err := testInstaller(t).Uninstall(root)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !strings.Contains(strings.Join(messages, "\n"), "pi is not connected to "+root) {
		t.Fatalf("messages = %q, want not-connected report", messages)
	}
}

func TestTrustDecided(t *testing.T) {
	writeTrust := func(t *testing.T, home, content string) {
		t.Helper()
		dir := filepath.Join(home, ".pi", "agent")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "trust.json"), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	cases := []struct {
		name    string
		root    string
		trust   string
		decided bool
	}{
		{name: "no trust file", root: "/a/b", trust: "", decided: false},
		{name: "exact decision trusts", root: "/a/b", trust: `{"/a/b": true}`, decided: true},
		{name: "parent decision trusts", root: "/a/b/c", trust: `{"/a/b": true}`, decided: true},
		{name: "exact decision declines", root: "/a/b", trust: `{"/a/b": false}`, decided: false},
		{name: "closest decision wins", root: "/a/b/c", trust: `{"/a": true, "/a/b": false}`, decided: false},
		{name: "unrelated decision", root: "/a/b", trust: `{"/x": true}`, decided: false},
		{name: "malformed trust file", root: "/a/b", trust: `{`, decided: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			if tc.trust != "" {
				writeTrust(t, home, tc.trust)
			}
			t.Setenv("HOME", home)
			if got := trustDecided(tc.root); got != tc.decided {
				t.Fatalf("trustDecided(%q) = %v, want %v", tc.root, got, tc.decided)
			}
		})
	}
}

func TestExtensionSourceHasNoRuntimeDependencies(t *testing.T) {
	src := currentExtensionSource("/tmp/project")
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import ") && !strings.HasPrefix(trimmed, "import type ") {
			t.Fatalf("runtime import found: %q", trimmed)
		}
	}
	if !strings.Contains(src, `import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";`) {
		t.Fatalf("type-only ExtensionAPI import missing")
	}
}

func TestExtensionSourceImplementsDeliveryContract(t *testing.T) {
	src := currentExtensionSource("/tmp/project")
	for _, want := range []string{
		`pi.on("before_agent_start"`,
		`pi.exec("learning-loop", ["render", projectRoot])`,
		"systemPrompt: event.systemPrompt",
		"ctx.ui.notify",
		"ctx?.hasUI",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("extension source missing %q", want)
		}
	}
}

func TestExtensionSourceEmbedsProjectRoot(t *testing.T) {
	root := "/abs/project"
	src := currentExtensionSource(root)
	if !strings.Contains(src, `const projectRoot = "/abs/project";`) {
		t.Fatalf("project root not embedded: %s", src)
	}
	if strings.Contains(src, projectRootPlaceholder) {
		t.Fatalf("placeholder leaked into generated source")
	}
}
