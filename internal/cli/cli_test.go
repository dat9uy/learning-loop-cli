package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dat9uy/learning-loop-cli/internal/recordstore"
)

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = Run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestInitCreatesRecordStore(t *testing.T) {
	root := t.TempDir()
	code, stdout, stderr := run(t, "init", root)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "initialized Record Store") {
		t.Fatalf("stdout = %q, want confirmation", stdout)
	}
	info, err := os.Stat(filepath.Join(root, recordstore.DirName, recordstore.RulesDirName))
	if err != nil || !info.IsDir() {
		t.Fatalf("Record Store not created: %v", err)
	}
}

func TestInitIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if code, _, stderr := run(t, "init", root); code != 0 {
		t.Fatalf("first init exit code = %d (stderr: %s)", code, stderr)
	}
	code, _, stderr := run(t, "init", root)
	if code != 0 {
		t.Fatalf("second init exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
}

func TestInitRequiresAbsoluteRoot(t *testing.T) {
	code, stdout, stderr := run(t, "init", "relative/path")
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "E100") {
		t.Fatalf("stderr = %q, want stable code E100", stderr)
	}
}

func TestRenderSuccessWritesOnlyMarkdown(t *testing.T) {
	root := t.TempDir()
	store := recordstore.New(root)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := store.CreateRevision("beta", []byte("---\nname: beta\n---\nbeta body\n")); err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	if _, err := store.CreateRevision("alpha", []byte("---\nname: alpha\n---\nalpha body\n")); err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	code, stdout, stderr := run(t, "render", root)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	want := "alpha body\n\nbeta body\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRenderFailureWritesNothingToStdout(t *testing.T) {
	root := t.TempDir()
	store := recordstore.New(root)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := store.CreateRevision("alpha", []byte("no frontmatter\n")); err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	code, stdout, stderr := run(t, "render", root)
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "E103") {
		t.Fatalf("stderr = %q, want stable code E103", stderr)
	}
}

func TestRenderUninitializedProjectFails(t *testing.T) {
	code, stdout, stderr := run(t, "render", t.TempDir())
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "E101") {
		t.Fatalf("stderr = %q, want stable code E101", stderr)
	}
}

func TestRenderEmptyStoreSucceedsWithEmptyStdout(t *testing.T) {
	root := t.TempDir()
	if err := recordstore.New(root).Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	code, stdout, stderr := run(t, "render", root)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestRenderUnavailableInputFails(t *testing.T) {
	root := t.TempDir()
	store := recordstore.New(root)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	ruleDir := filepath.Join(root, recordstore.DirName, recordstore.RulesDirName, "alpha")
	if err := os.MkdirAll(ruleDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink("nowhere", filepath.Join(ruleDir, "0001.md")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	code, stdout, stderr := run(t, "render", root)
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "E105") {
		t.Fatalf("stderr = %q, want stable code E105", stderr)
	}
}

func TestRenderRequiresAbsoluteRoot(t *testing.T) {
	code, stdout, stderr := run(t, "render", "relative/path")
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "E100") {
		t.Fatalf("stderr = %q, want stable code E100", stderr)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"bogus"}, {"render"}, {"init"}} {
		code, stdout, stderr := run(t, args...)
		if code == 0 {
			t.Fatalf("args %v: exit code = 0, want nonzero", args)
		}
		if stdout != "" {
			t.Fatalf("args %v: stdout = %q, want empty", args, stdout)
		}
		if !strings.Contains(stderr, "Usage:") {
			t.Fatalf("args %v: stderr = %q, want usage", args, stderr)
		}
	}
}
