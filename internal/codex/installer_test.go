package codex

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
	d.codexVersion = func() (string, error) { return "codex-cli 0.147.0\n", nil }
	for _, o := range opts {
		o(&d)
	}
	return &installer{deps: d}
}

func readHooks(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(hooksPath(root))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(data)
}

func writeHooks(t *testing.T, root, content string) {
	t.Helper()
	dir := filepath.Join(root, HooksDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(hooksPath(root), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var cerr *Error
	if errors.As(err, &cerr) {
		if cerr.Code != code {
			t.Fatalf("error code = %s, want %s (%v)", cerr.Code, code, cerr)
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
	t.Fatalf("error = %v, want *Error with code %s", err, code)
}

func TestInstallCreatesExactShapeAndReportsTrust(t *testing.T) {
	root := t.TempDir()
	messages, err := testInstaller(t).Install(root)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	want := `{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "command": "learning-loop codex-adapter",
            "statusMessage": "learning-loop: delivering project Rules",
            "type": "command"
          }
        ]
      }
    ]
  }
}
`
	if got := readHooks(t, root); got != want {
		t.Fatalf("hooks.json = %q, want %q", got, want)
	}
	joined := strings.Join(messages, "\n")
	if !strings.Contains(joined, "connected Codex to "+root) {
		t.Fatalf("messages = %q, want connection confirmation", joined)
	}
	if !strings.Contains(joined, "hook trust is pending") {
		t.Fatalf("messages = %q, want trust pending report", joined)
	}
	if !strings.Contains(joined, "hook management surface") {
		t.Fatalf("messages = %q, want direction to Codex's hook management surface", joined)
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	root := t.TempDir()
	inst := testInstaller(t)
	if _, err := inst.Install(root); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	before := readHooks(t, root)
	beforeInfo, err := os.Stat(hooksPath(root))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if _, err := inst.Install(root); err != nil {
		t.Fatalf("second Install: %v", err)
	}
	after := readHooks(t, root)
	if after != before {
		t.Fatalf("second Install rewrote hooks.json: %q != %q", after, before)
	}
	afterInfo, err := os.Stat(hooksPath(root))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Fatalf("second Install rewrote hooks.json (modtime changed)")
	}
	if !os.SameFile(beforeInfo, afterInfo) {
		t.Fatalf("second Install rewrote hooks.json (inode changed)")
	}
}

func TestInstallUpgradesRecognizedOlderShape(t *testing.T) {
	root := t.TempDir()
	writeHooks(t, root, `{
  "description": "workspace hooks",
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "learning-loop codex-adapter"
          }
        ]
      }
    ]
  }
}
`)
	if _, err := testInstaller(t).Install(root); err != nil {
		t.Fatalf("Install: %v", err)
	}
	got := readHooks(t, root)
	if !strings.Contains(got, `"statusMessage": "learning-loop: delivering project Rules"`) {
		t.Fatalf("older shape not upgraded: %s", got)
	}
	if !strings.Contains(got, `"description": "workspace hooks"`) {
		t.Fatalf("unrelated content not preserved: %s", got)
	}
}

func TestInstallPreservesUnrelatedConfiguration(t *testing.T) {
	root := t.TempDir()
	writeHooks(t, root, `{
  "description": "workspace hooks",
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume",
        "hooks": [
          {
            "type": "command",
            "command": "python3 ~/.codex/hooks/session_start.py",
            "statusMessage": "Loading session notes"
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "/usr/bin/python3 policy.py",
            "timeout": 10
          }
        ]
      }
    ]
  }
}
`)
	if _, err := testInstaller(t).Install(root); err != nil {
		t.Fatalf("Install: %v", err)
	}
	got := readHooks(t, root)
	for _, want := range []string{
		`"description": "workspace hooks"`,
		`"matcher": "startup|resume"`,
		`"command": "python3 ~/.codex/hooks/session_start.py"`,
		`"statusMessage": "Loading session notes"`,
		`"matcher": "Bash"`,
		`"command": "/usr/bin/python3 policy.py"`,
		`"timeout": 10`,
		`"command": "learning-loop codex-adapter"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("hooks.json missing %q: %s", want, got)
		}
	}
}

func TestInstallModifiedLearningLoopContentFailsUntouched(t *testing.T) {
	root := t.TempDir()
	writeHooks(t, root, `{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "learning-loop codex-adapter",
            "statusMessage": "edited by the user"
          }
        ]
      }
    ]
  }
}
`)
	before := readHooks(t, root)
	_, err := testInstaller(t).Install(root)
	wantCode(t, err, "E203")
	if got := readHooks(t, root); got != before {
		t.Fatalf("modified content was touched: %s", got)
	}
}

func TestInstallUnknownLearningLoopContentFailsUntouched(t *testing.T) {
	root := t.TempDir()
	writeHooks(t, root, `{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "learning-loop render /some/other/project"
          }
        ]
      }
    ]
  }
}
`)
	before := readHooks(t, root)
	_, err := testInstaller(t).Install(root)
	wantCode(t, err, "E203")
	if got := readHooks(t, root); got != before {
		t.Fatalf("unknown content was touched: %s", got)
	}
}

func TestInstallMalformedConfigFailsUntouched(t *testing.T) {
	root := t.TempDir()
	writeHooks(t, root, `{"hooks": {`)
	before := readHooks(t, root)
	_, err := testInstaller(t).Install(root)
	wantCode(t, err, "E202")
	if got := readHooks(t, root); got != before {
		t.Fatalf("malformed config was touched: %s", got)
	}
}

func TestInstallStructurallyInvalidConfigFails(t *testing.T) {
	for _, content := range []string{
		`{"hooks": "bogus"}`,
		`{"hooks": {"SessionStart": "bogus"}}`,
		`{"hooks": {"SessionStart": [{"hooks": "bogus"}]}}`,
		`{"hooks": {"SessionStart": [{"hooks": [42]}]}}`,
	} {
		root := t.TempDir()
		writeHooks(t, root, content)
		_, err := testInstaller(t).Install(root)
		wantCode(t, err, "E202")
	}
}

func TestInstallPathMismatchStopsWithRemediation(t *testing.T) {
	root := t.TempDir()
	inst := testInstaller(t, func(d *deps) {
		d.lookPath = func(string) (string, error) { return "/usr/local/bin/learning-loop", nil }
	})
	_, err := inst.Install(root)
	wantCode(t, err, "E201")
	if !strings.Contains(err.Error(), "PATH") {
		t.Fatalf("error = %v, want PATH remediation", err)
	}
	if _, statErr := os.Stat(hooksPath(root)); !os.IsNotExist(statErr) {
		t.Fatalf("hooks.json created despite PATH mismatch")
	}
}

func TestInstallPathResolvesToDifferentExecutableStops(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(t.TempDir(), "learning-loop")
	if err := os.WriteFile(other, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	inst := testInstaller(t, func(d *deps) {
		d.lookPath = func(string) (string, error) { return other, nil }
	})
	_, err := inst.Install(root)
	wantCode(t, err, "E201")
	if _, statErr := os.Stat(hooksPath(root)); !os.IsNotExist(statErr) {
		t.Fatalf("hooks.json created despite PATH mismatch")
	}
}

func TestInstallPathMissingStopsWithRemediation(t *testing.T) {
	root := t.TempDir()
	inst := testInstaller(t, func(d *deps) {
		d.lookPath = func(string) (string, error) { return "", errors.New("not found") }
	})
	_, err := inst.Install(root)
	wantCode(t, err, "E201")
	if !strings.Contains(err.Error(), "PATH") {
		t.Fatalf("error = %v, want PATH remediation", err)
	}
}

func TestInstallWarnsForDifferentCodexVersion(t *testing.T) {
	root := t.TempDir()
	inst := testInstaller(t, func(d *deps) {
		d.codexVersion = func() (string, error) { return "codex-cli 0.123.0\n", nil }
	})
	messages, err := inst.Install(root)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	joined := strings.Join(messages, "\n")
	if !strings.Contains(joined, "warning: Codex 0.123.0 is not the validated 0.147.0") {
		t.Fatalf("messages = %q, want version warning", joined)
	}
	if !strings.Contains(joined, "connected Codex to "+root) {
		t.Fatalf("messages = %q, want connection despite warning", joined)
	}
}

func TestInstallWarnsWhenCodexVersionUndetectable(t *testing.T) {
	root := t.TempDir()
	inst := testInstaller(t, func(d *deps) {
		d.codexVersion = func() (string, error) { return "", errors.New("not found") }
	})
	messages, err := inst.Install(root)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !strings.Contains(strings.Join(messages, "\n"), "warning: could not detect the Codex version") {
		t.Fatalf("messages = %q, want undetectable warning", messages)
	}
}

func TestInstallNoWarningForValidatedVersion(t *testing.T) {
	root := t.TempDir()
	messages, err := testInstaller(t).Install(root)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if strings.Contains(strings.Join(messages, "\n"), "warning:") {
		t.Fatalf("messages = %q, want no warning for validated version", messages)
	}
}

func TestUninstallRemovesOnlyLearningLoopContent(t *testing.T) {
	root := t.TempDir()
	writeHooks(t, root, `{
  "description": "workspace hooks",
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume",
        "hooks": [
          {
            "type": "command",
            "command": "python3 ~/.codex/hooks/session_start.py"
          }
        ]
      },
      {
        "hooks": [
          {
            "type": "command",
            "command": "learning-loop codex-adapter",
            "statusMessage": "learning-loop: delivering project Rules"
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "/usr/bin/python3 policy.py"
          }
        ]
      }
    ]
  }
}
`)
	messages, err := testInstaller(t).Uninstall(root)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !strings.Contains(strings.Join(messages, "\n"), "disconnected Codex from "+root) {
		t.Fatalf("messages = %q, want disconnection confirmation", messages)
	}
	got := readHooks(t, root)
	if strings.Contains(got, "learning-loop") {
		t.Fatalf("learning-loop content remains: %s", got)
	}
	for _, want := range []string{
		`"description": "workspace hooks"`,
		`"matcher": "startup|resume"`,
		`"command": "python3 ~/.codex/hooks/session_start.py"`,
		`"matcher": "Bash"`,
		`"command": "/usr/bin/python3 policy.py"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("hooks.json missing %q: %s", want, got)
		}
	}
}

func TestUninstallRemovesFileWhenDocumentBecomesEmpty(t *testing.T) {
	root := t.TempDir()
	writeHooks(t, root, `{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "learning-loop codex-adapter",
            "statusMessage": "learning-loop: delivering project Rules"
          }
        ]
      }
    ]
  }
}
`)
	if _, err := testInstaller(t).Uninstall(root); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if _, err := os.Stat(hooksPath(root)); !os.IsNotExist(err) {
		t.Fatalf("hooks.json still exists after uninstall")
	}
}

func TestUninstallRemovesRecognizedOlderShape(t *testing.T) {
	root := t.TempDir()
	writeHooks(t, root, `{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "learning-loop codex-adapter"
          }
        ]
      }
    ]
  }
}
`)
	if _, err := testInstaller(t).Uninstall(root); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if _, err := os.Stat(hooksPath(root)); !os.IsNotExist(err) {
		t.Fatalf("hooks.json still exists after uninstall")
	}
}

func TestUninstallNotConnectedIsIdempotent(t *testing.T) {
	root := t.TempDir()
	messages, err := testInstaller(t).Uninstall(root)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !strings.Contains(strings.Join(messages, "\n"), "Codex is not connected to "+root) {
		t.Fatalf("messages = %q, want not-connected report", messages)
	}
}

func TestUninstallNotConnectedWithUnrelatedConfig(t *testing.T) {
	root := t.TempDir()
	writeHooks(t, root, `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "/usr/bin/python3 policy.py"
          }
        ]
      }
    ]
  }
}
`)
	before := readHooks(t, root)
	messages, err := testInstaller(t).Uninstall(root)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !strings.Contains(strings.Join(messages, "\n"), "Codex is not connected to "+root) {
		t.Fatalf("messages = %q, want not-connected report", messages)
	}
	if got := readHooks(t, root); got != before {
		t.Fatalf("unrelated config was touched: %s", got)
	}
}

func TestUninstallModifiedContentFailsUntouched(t *testing.T) {
	root := t.TempDir()
	writeHooks(t, root, `{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "learning-loop codex-adapter",
            "statusMessage": "edited by the user"
          }
        ]
      }
    ]
  }
}
`)
	before := readHooks(t, root)
	_, err := testInstaller(t).Uninstall(root)
	wantCode(t, err, "E203")
	if got := readHooks(t, root); got != before {
		t.Fatalf("modified content was touched: %s", got)
	}
}

func TestUninstallMalformedConfigFailsUntouched(t *testing.T) {
	root := t.TempDir()
	writeHooks(t, root, `{"hooks": {`)
	before := readHooks(t, root)
	_, err := testInstaller(t).Uninstall(root)
	wantCode(t, err, "E202")
	if got := readHooks(t, root); got != before {
		t.Fatalf("malformed config was touched: %s", got)
	}
}

func TestInstallWriteFailureLeavesOriginalIntact(t *testing.T) {
	root := t.TempDir()
	writeHooks(t, root, `{"description": "workspace hooks"}`)
	dir := filepath.Join(root, HooksDirName)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	defer os.Chmod(dir, 0o755)
	before := readHooks(t, root)
	_, err := testInstaller(t).Install(root)
	wantCode(t, err, "E204")
	if got := readHooks(t, root); got != before {
		t.Fatalf("original config not preserved after failed write: %s", got)
	}
}

func TestInstallPreservesNumberLiterals(t *testing.T) {
	root := t.TempDir()
	writeHooks(t, root, `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "/usr/bin/python3 policy.py",
            "timeout": 1e3
          }
        ]
      }
    ]
  }
}
`)
	if _, err := testInstaller(t).Install(root); err != nil {
		t.Fatalf("Install: %v", err)
	}
	got := readHooks(t, root)
	if !strings.Contains(got, `"timeout": 1e3`) {
		t.Fatalf("number literal not preserved: %s", got)
	}
}

func TestInstallRequiresAbsoluteRoot(t *testing.T) {
	_, err := testInstaller(t).Install("relative/path")
	wantCode(t, err, "E100")
}

func TestUninstallRequiresAbsoluteRoot(t *testing.T) {
	_, err := testInstaller(t).Uninstall("relative/path")
	wantCode(t, err, "E100")
}
