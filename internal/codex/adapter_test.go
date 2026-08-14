package codex

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dat9uy/learning-loop-cli/internal/recordstore"
)

func runAdapter(t *testing.T, stdin string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = RunAdapter(strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func sessionStartEventJSON(t *testing.T, cwd string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"cwd":             cwd,
		"hook_event_name": "SessionStart",
		"model":           "o4-mini",
		"permission_mode": "default",
		"session_id":      "session-1",
		"source":          "startup",
		"transcript_path": nil,
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return string(b)
}

func newProjectWithRule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	store := recordstore.New(root)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := store.CreateRevision("alpha", []byte("---\nname: alpha\n---\nalpha body\n")); err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	return root
}

func decodeEnvelope(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout %q is not valid JSON: %v", stdout, err)
	}
	return env
}

func TestAdapterEmitsAdditionalContextEnvelope(t *testing.T) {
	root := newProjectWithRule(t)
	code, stdout, stderr := runAdapter(t, sessionStartEventJSON(t, root))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	env := decodeEnvelope(t, stdout)
	hso, _ := env["hookSpecificOutput"].(map[string]any)
	if hso == nil {
		t.Fatalf("envelope missing hookSpecificOutput: %s", stdout)
	}
	if hso["hookEventName"] != "SessionStart" {
		t.Fatalf("hookEventName = %v, want SessionStart", hso["hookEventName"])
	}
	if hso["additionalContext"] != "alpha body\n" {
		t.Fatalf("additionalContext = %q, want rendered Instruction", hso["additionalContext"])
	}
	if _, ok := env["systemMessage"]; ok {
		t.Fatalf("envelope carries systemMessage on success: %s", stdout)
	}
}

func TestAdapterEmptyProjectEmitsNoAdditionalContext(t *testing.T) {
	root := t.TempDir()
	if err := recordstore.New(root).Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	code, stdout, stderr := runAdapter(t, sessionStartEventJSON(t, root))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	env := decodeEnvelope(t, stdout)
	hso, _ := env["hookSpecificOutput"].(map[string]any)
	if hso == nil {
		t.Fatalf("envelope missing hookSpecificOutput: %s", stdout)
	}
	if _, ok := hso["additionalContext"]; ok {
		t.Fatalf("additionalContext present for empty projection: %s", stdout)
	}
}

func TestAdapterRenderFailureFailsOpen(t *testing.T) {
	root := t.TempDir()
	code, stdout, stderr := runAdapter(t, sessionStartEventJSON(t, root))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (fail open)", code)
	}
	env := decodeEnvelope(t, stdout)
	if _, ok := env["hookSpecificOutput"]; ok {
		t.Fatalf("failure emitted an Instruction envelope: %s", stdout)
	}
	msg, _ := env["systemMessage"].(string)
	if !strings.Contains(msg, "E101") {
		t.Fatalf("systemMessage = %q, want stable code E101", msg)
	}
	if !strings.Contains(stderr, "E101") {
		t.Fatalf("stderr = %q, want stable code E101", stderr)
	}
}

func TestAdapterMalformedEventFailsOpen(t *testing.T) {
	code, stdout, stderr := runAdapter(t, "not json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (fail open)", code)
	}
	env := decodeEnvelope(t, stdout)
	if _, ok := env["hookSpecificOutput"]; ok {
		t.Fatalf("failure emitted an Instruction envelope: %s", stdout)
	}
	if !strings.Contains(env["systemMessage"].(string), "E205") {
		t.Fatalf("systemMessage = %v, want stable code E205", env["systemMessage"])
	}
	if !strings.Contains(stderr, "E205") {
		t.Fatalf("stderr = %q, want stable code E205", stderr)
	}
}

func TestAdapterWrongEventNameFailsOpen(t *testing.T) {
	root := t.TempDir()
	event := strings.Replace(sessionStartEventJSON(t, root), `"hook_event_name":"SessionStart"`, `"hook_event_name":"UserPromptSubmit"`, 1)
	code, stdout, _ := runAdapter(t, event)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (fail open)", code)
	}
	env := decodeEnvelope(t, stdout)
	if _, ok := env["hookSpecificOutput"]; ok {
		t.Fatalf("failure emitted an Instruction envelope: %s", stdout)
	}
	if !strings.Contains(env["systemMessage"].(string), "E206") {
		t.Fatalf("systemMessage = %v, want stable code E206", env["systemMessage"])
	}
}

func TestAdapterRelativeCwdFailsOpen(t *testing.T) {
	event := strings.Replace(sessionStartEventJSON(t, "/abs/path"), `"cwd":"/abs/path"`, `"cwd":"relative/path"`, 1)
	code, stdout, _ := runAdapter(t, event)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (fail open)", code)
	}
	env := decodeEnvelope(t, stdout)
	if !strings.Contains(env["systemMessage"].(string), "E206") {
		t.Fatalf("systemMessage = %v, want stable code E206", env["systemMessage"])
	}
}

func TestAdapterEmptyStdinFailsOpen(t *testing.T) {
	code, stdout, _ := runAdapter(t, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (fail open)", code)
	}
	env := decodeEnvelope(t, stdout)
	if !strings.Contains(env["systemMessage"].(string), "E205") {
		t.Fatalf("systemMessage = %v, want stable code E205", env["systemMessage"])
	}
}
