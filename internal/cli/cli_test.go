package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dat9uy/learning-loop-cli/internal/recordstore"
	"github.com/dat9uy/learning-loop-cli/internal/runtimecache"
)

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	return runStdin(t, "", args...)
}

func runStdin(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = Run(args, strings.NewReader(stdin), &out, &errOut)
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
	for _, args := range [][]string{nil, {"bogus"}, {"render"}, {"init"}, {"connect"}, {"connect", "unknown", "/tmp/x"}, {"disconnect"}, {"disconnect", "unknown", "/tmp/x"}, {"codex-adapter", "extra"}} {
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

func fakePathEnv(t *testing.T, codexVersion string) string {
	t.Helper()
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable: %v", err)
	}
	if err := os.Symlink(exe, filepath.Join(dir, "learning-loop")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	script := filepath.Join(dir, "codex")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho "+codexVersion+"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return dir
}

func TestConnectCodexCreatesHook(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", fakePathEnv(t, "codex-cli 0.147.0")+string(os.PathListSeparator)+os.Getenv("PATH"))
	code, stdout, stderr := run(t, "connect", "codex", root)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "connected Codex to "+root) {
		t.Fatalf("stdout = %q, want connection confirmation", stdout)
	}
	if !strings.Contains(stdout, "hook trust is pending") {
		t.Fatalf("stdout = %q, want trust pending report", stdout)
	}
	data, err := os.ReadFile(filepath.Join(root, ".codex", "hooks.json"))
	if err != nil {
		t.Fatalf("hooks.json: %v", err)
	}
	if !strings.Contains(string(data), "learning-loop codex-adapter") {
		t.Fatalf("hooks.json = %q, want learning-loop handler", data)
	}
}

func fakeOpenCodePathEnv(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable: %v", err)
	}
	if err := os.Symlink(exe, filepath.Join(dir, "learning-loop")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	script := filepath.Join(dir, "opencode")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho opencode "+version+"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return dir
}

func TestConnectOpenCodeCreatesPlugin(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", fakeOpenCodePathEnv(t, "1.18.18")+string(os.PathListSeparator)+os.Getenv("PATH"))
	code, stdout, stderr := run(t, "connect", "opencode", root)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "connected OpenCode to "+root) {
		t.Fatalf("stdout = %q, want connection confirmation", stdout)
	}
	data, err := os.ReadFile(filepath.Join(root, ".opencode", "plugins", "learning-loop.js"))
	if err != nil {
		t.Fatalf("learning-loop.js: %v", err)
	}
	if !strings.Contains(string(data), `"experimental.chat.system.transform"`) {
		t.Fatalf("learning-loop.js = %q, want native system transform hook", data)
	}
}

func TestConnectOpenCodeWarnsForDifferentVersion(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", fakeOpenCodePathEnv(t, "1.19.0")+string(os.PathListSeparator)+os.Getenv("PATH"))
	code, stdout, stderr := run(t, "connect", "opencode", root)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "warning: OpenCode 1.19.0 is not the validated 1.18.18") {
		t.Fatalf("stdout = %q, want version warning", stdout)
	}
}

func TestConnectOpenCodePathMismatchFails(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	code, stdout, stderr := run(t, "connect", "opencode", root)
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "E201") {
		t.Fatalf("stderr = %q, want stable code E201", stderr)
	}
}

func TestDisconnectOpenCodeRemovesPlugin(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", fakeOpenCodePathEnv(t, "1.18.18")+string(os.PathListSeparator)+os.Getenv("PATH"))
	if code, _, stderr := run(t, "connect", "opencode", root); code != 0 {
		t.Fatalf("connect exit code = %d (stderr: %s)", code, stderr)
	}
	code, stdout, stderr := run(t, "disconnect", "opencode", root)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "disconnected OpenCode from "+root) {
		t.Fatalf("stdout = %q, want disconnection confirmation", stdout)
	}
	if _, err := os.Stat(filepath.Join(root, ".opencode", "plugins", "learning-loop.js")); !os.IsNotExist(err) {
		t.Fatalf("learning-loop.js still exists")
	}
}

func TestConnectCodexWarnsForDifferentVersion(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", fakePathEnv(t, "codex-cli 0.123.0")+string(os.PathListSeparator)+os.Getenv("PATH"))
	code, stdout, stderr := run(t, "connect", "codex", root)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "warning: Codex 0.123.0 is not the validated 0.147.0") {
		t.Fatalf("stdout = %q, want version warning", stdout)
	}
}

func TestConnectCodexPathMismatchFails(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	code, stdout, stderr := run(t, "connect", "codex", root)
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "E201") {
		t.Fatalf("stderr = %q, want stable code E201", stderr)
	}
}

func TestDisconnectCodexRemovesHook(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", fakePathEnv(t, "codex-cli 0.147.0")+string(os.PathListSeparator)+os.Getenv("PATH"))
	if code, _, stderr := run(t, "connect", "codex", root); code != 0 {
		t.Fatalf("connect exit code = %d (stderr: %s)", code, stderr)
	}
	code, stdout, stderr := run(t, "disconnect", "codex", root)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "disconnected Codex from "+root) {
		t.Fatalf("stdout = %q, want disconnection confirmation", stdout)
	}
	if _, err := os.Stat(filepath.Join(root, ".codex", "hooks.json")); !os.IsNotExist(err) {
		t.Fatalf("hooks.json still exists after disconnect")
	}
}

func TestCodexAdapterCommandEmitsEnvelope(t *testing.T) {
	root := t.TempDir()
	store := recordstore.New(root)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := store.CreateRevision("alpha", []byte("---\nname: alpha\n---\nalpha body\n")); err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	event := fmt.Sprintf(`{"cwd":%q,"hook_event_name":"SessionStart","model":"o4-mini","permission_mode":"default","session_id":"s","source":"startup","transcript_path":null}`, root)
	code, stdout, stderr := runStdin(t, event, "codex-adapter")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, `"hookEventName":"SessionStart"`) {
		t.Fatalf("stdout = %q, want native envelope", stdout)
	}
	if !strings.Contains(stdout, `"additionalContext":"alpha body\n"`) {
		t.Fatalf("stdout = %q, want rendered Instruction in additionalContext", stdout)
	}
}

// populateRuntimeCache writes a fake cached Runtime executable with a
// matching recorded checksum into the given cache directory.
func populateRuntimeCache(t *testing.T, cacheDir, name, version, script string) {
	t.Helper()
	target := filepath.Join(cacheDir, name+"-"+version)
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	bin := filepath.Join(target, name)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	sum := sha256.Sum256([]byte(script))
	if err := os.WriteFile(bin+".sha256", []byte(hex.EncodeToString(sum[:])+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile checksum: %v", err)
	}
}

func TestConformanceCodexMissingCacheReportsRemediation(t *testing.T) {
	t.Setenv("LEARNING_LOOP_CACHE", t.TempDir())
	code, stdout, stderr := run(t, "conformance", "codex")
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "E300") {
		t.Fatalf("stderr = %q, want stable code E300", stderr)
	}
	if !strings.Contains(stderr, "learning-loop runtime-setup codex") {
		t.Fatalf("stderr = %q, want the exact setup remediation", stderr)
	}
}

func TestConformanceCodexFailurePrintsSanitizedBundle(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", cacheDir)
	populateRuntimeCache(t, cacheDir, "codex", runtimecache.CodexVersion, "#!/bin/sh\nexit 0\n")

	code, _, stderr := run(t, "conformance", "codex")
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero")
	}
	if !strings.Contains(stderr, "conformance codex: FAIL") {
		t.Fatalf("stderr = %q, want FAIL banner", stderr)
	}
	if !strings.Contains(stderr, "pinned runtime: codex "+runtimecache.CodexVersion) {
		t.Fatalf("stderr = %q, want the pinned version", stderr)
	}
	if !strings.Contains(stderr, "outbound model requests: 0") {
		t.Fatalf("stderr = %q, want the request count", stderr)
	}
	if !strings.Contains(stderr, "launch arguments:") {
		t.Fatalf("stderr = %q, want the launch arguments", stderr)
	}
	if !strings.Contains(stderr, "runtime stdout:") || !strings.Contains(stderr, "runtime stderr:") {
		t.Fatalf("stderr = %q, want bounded runtime output", stderr)
	}
	if strings.Contains(stderr, "OPENAI_API_KEY") {
		t.Fatalf("stderr = %q, want sanitized output", stderr)
	}
}

func TestRuntimeSetupCodexIdempotentWithValidCache(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", cacheDir)
	populateRuntimeCache(t, cacheDir, "codex", runtimecache.CodexVersion, "#!/bin/sh\necho fake codex\n")

	code, stdout, stderr := run(t, "runtime-setup", "codex")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "cached at") {
		t.Fatalf("stdout = %q, want cache confirmation", stdout)
	}
}

func TestConformanceOpenCodeMissingCacheReportsRemediation(t *testing.T) {
	t.Setenv("LEARNING_LOOP_CACHE", t.TempDir())
	code, stdout, stderr := run(t, "conformance", "opencode")
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "E300") {
		t.Fatalf("stderr = %q, want stable code E300", stderr)
	}
	if !strings.Contains(stderr, "learning-loop runtime-setup opencode") {
		t.Fatalf("stderr = %q, want the exact setup remediation", stderr)
	}
}

func TestConformanceOpenCodeFailurePrintsSanitizedBundle(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", cacheDir)
	populateRuntimeCache(t, cacheDir, "opencode", runtimecache.OpenCodeVersion, "#!/bin/sh\nexit 0\n")

	code, _, stderr := run(t, "conformance", "opencode")
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero")
	}
	if !strings.Contains(stderr, "conformance opencode: FAIL") {
		t.Fatalf("stderr = %q, want FAIL banner", stderr)
	}
	if !strings.Contains(stderr, "pinned runtime: opencode "+runtimecache.OpenCodeVersion) {
		t.Fatalf("stderr = %q, want the pinned version", stderr)
	}
	if !strings.Contains(stderr, "outbound model requests: 0") {
		t.Fatalf("stderr = %q, want the request count", stderr)
	}
	if !strings.Contains(stderr, "launch arguments:") {
		t.Fatalf("stderr = %q, want the launch arguments", stderr)
	}
	if !strings.Contains(stderr, "runtime stdout:") || !strings.Contains(stderr, "runtime stderr:") {
		t.Fatalf("stderr = %q, want bounded runtime output", stderr)
	}
	if strings.Contains(stderr, "apiKey") {
		t.Fatalf("stderr = %q, want sanitized output", stderr)
	}
}

func TestRuntimeSetupOpenCodeIdempotentWithValidCache(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", cacheDir)
	populateRuntimeCache(t, cacheDir, "opencode", runtimecache.OpenCodeVersion, "#!/bin/sh\necho fake opencode\n")

	code, stdout, stderr := run(t, "runtime-setup", "opencode")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "cached at") {
		t.Fatalf("stdout = %q, want cache confirmation", stdout)
	}
}

func TestRuntimeSelectionAcceptsEitherOrder(t *testing.T) {
	got, err := selectRuntimes([]string{"opencode", "codex"})
	if err != nil {
		t.Fatalf("selectRuntimes: %v", err)
	}
	want := []string{"codex", "opencode"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("selected = %v, want %v", got, want)
	}
	for _, args := range [][]string{{"codex", "codex"}, {"opencode", "unknown"}} {
		if _, err := selectRuntimes(args); err == nil {
			t.Fatalf("selectRuntimes(%v) succeeded, want validation error", args)
		}
	}
}

func TestRuntimeSetupCombinedLeavesValidEntriesUntouched(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", cacheDir)
	populateRuntimeCache(t, cacheDir, "codex", runtimecache.CodexVersion, "#!/bin/sh\necho codex\n")
	populateRuntimeCache(t, cacheDir, "opencode", runtimecache.OpenCodeVersion, "#!/bin/sh\necho opencode\n")

	code, stdout, stderr := run(t, "runtime-setup", "opencode", "codex")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	codexAt := strings.Index(stdout, "Codex "+runtimecache.CodexVersion)
	opencodeAt := strings.Index(stdout, "OpenCode "+runtimecache.OpenCodeVersion)
	if codexAt < 0 || opencodeAt < 0 || codexAt > opencodeAt {
		t.Fatalf("stdout = %q, want both cache confirmations in canonical order", stdout)
	}
}

func TestCombinedConformancePreflightsAllRuntimes(t *testing.T) {
	cacheDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "codex-launched")
	t.Setenv("LEARNING_LOOP_CACHE", cacheDir)
	populateRuntimeCache(t, cacheDir, "codex", runtimecache.CodexVersion, "#!/bin/sh\ntouch "+marker+"\nexit 0\n")

	code, stdout, stderr := run(t, "conformance", "codex", "opencode")
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want no case output", stdout)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("Codex launched despite an invalid OpenCode prerequisite")
	}
	if !strings.Contains(stderr, "learning-loop runtime-setup opencode") {
		t.Fatalf("stderr = %q, want the exact OpenCode repair command", stderr)
	}
}

func TestCombinedConformanceRunsBothAndEmitsCanonicalOrder(t *testing.T) {
	cacheDir := t.TempDir()
	codexMarker := filepath.Join(t.TempDir(), "codex-launched")
	openCodeMarker := filepath.Join(t.TempDir(), "opencode-launched")
	t.Setenv("LEARNING_LOOP_CACHE", cacheDir)
	populateRuntimeCache(t, cacheDir, "codex", runtimecache.CodexVersion, "#!/bin/sh\ntouch "+codexMarker+"\nexit 0\n")
	populateRuntimeCache(t, cacheDir, "opencode", runtimecache.OpenCodeVersion, "#!/bin/sh\ntouch "+openCodeMarker+"\nexit 0\n")

	code, _, stderr := run(t, "conformance", "opencode", "codex")
	if code == 0 {
		t.Fatalf("exit code = 0, want both fake cases to fail their semantic checks")
	}
	for _, marker := range []string{codexMarker, openCodeMarker} {
		if _, err := os.Stat(marker); err != nil {
			t.Fatalf("case did not launch: %s: %v", marker, err)
		}
	}
	codexAt := strings.Index(stderr, "conformance codex: FAIL")
	opencodeAt := strings.Index(stderr, "conformance opencode: FAIL")
	if codexAt < 0 || opencodeAt < 0 || codexAt > opencodeAt {
		t.Fatalf("stderr = %q, want failures in canonical order", stderr)
	}
}

func TestStandaloneAndCombinedKeepRetainState(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", cacheDir)
	for _, name := range []string{"codex", "opencode"} {
		version := runtimecache.CodexVersion
		if name == "opencode" {
			version = runtimecache.OpenCodeVersion
		}
		populateRuntimeCache(t, cacheDir, name, version, "#!/bin/sh\nexit 0\n")
	}

	code, stdout, stderr := run(t, "conformance", "opencode", "codex", "--keep")
	if code == 0 {
		t.Fatalf("exit code = 0, want fake cases to fail semantic checks")
	}
	if strings.Count(stdout, "retaining full state at ") != 2 {
		t.Fatalf("stdout = %q, want both retained-state paths", stdout)
	}
	if stderr == "" {
		t.Fatalf("stderr = %q, want failure bundles", stderr)
	}
	for _, line := range strings.Split(stdout, "\n") {
		if !strings.Contains(line, "retaining full state at ") {
			continue
		}
		path := line[strings.Index(line, "retaining full state at ")+len("retaining full state at "):]
		if err := os.RemoveAll(path); err != nil {
			t.Fatalf("remove retained state %q: %v", path, err)
		}
	}

	code, stdout, stderr = run(t, "conformance", "codex", "--keep")
	if code == 0 || !strings.Contains(stdout, "retaining full state at ") || stderr == "" {
		t.Fatalf("standalone keep: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "retaining full state at ") {
			path := line[strings.Index(line, "retaining full state at ")+len("retaining full state at "):]
			_ = os.RemoveAll(path)
		}
	}
}

func TestConformanceUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"conformance"}, {"conformance", "bogus"}, {"conformance", "codex", "--bogus"},
		{"conformance", "codex", "opencode", "opencode"}, {"conformance", "codex", "--keep", "opencode"},
		{"runtime-setup"}, {"runtime-setup", "bogus"}, {"runtime-setup", "codex", "codex"},
	} {
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
