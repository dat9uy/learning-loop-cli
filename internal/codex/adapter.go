package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/dat9uy/learning-loop-cli/internal/render"
)

// sessionStartEvent is the subset of Codex's native SessionStart hook input
// that the Adapter consumes.
type sessionStartEvent struct {
	Cwd           string `json:"cwd"`
	HookEventName string `json:"hook_event_name"`
}

// RunAdapter executes the Codex SessionStart hook Adapter: it reads the
// native hook event from stdin, obtains the project from the event, calls
// the shared renderer in-process, and emits Codex's native
// additional-context envelope. It never spawns the raw render subcommand.
// Every failure emits no Instruction, reports the stable diagnostic outside
// model context, and allows Codex to continue.
func RunAdapter(stdin io.Reader, stdout, stderr io.Writer) int {
	data, err := io.ReadAll(stdin)
	if err != nil {
		emitDiagnostic(stdout, stderr, "E205", fmt.Sprintf("reading the Codex event: %v", err))
		return 0
	}
	var ev sessionStartEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		emitDiagnostic(stdout, stderr, "E205", fmt.Sprintf("decoding the Codex SessionStart event: %v", err))
		return 0
	}
	if ev.HookEventName != "SessionStart" {
		emitDiagnostic(stdout, stderr, "E206", fmt.Sprintf("unexpected hook event %q", ev.HookEventName))
		return 0
	}
	if ev.Cwd == "" || !filepath.IsAbs(ev.Cwd) {
		emitDiagnostic(stdout, stderr, "E206", fmt.Sprintf("event cwd %q is not an absolute project path", ev.Cwd))
		return 0
	}
	out, err := render.New().Render(ev.Cwd)
	if err != nil {
		code, msg := adapterDiagnostic(err)
		emitDiagnostic(stdout, stderr, code, msg)
		return 0
	}
	emitContext(stdout, string(out))
	return 0
}

// adapterDiagnostic maps a rendering failure to its stable diagnostic code.
func adapterDiagnostic(err error) (code, msg string) {
	var rerr *render.Error
	if errors.As(err, &rerr) {
		return rerr.Code, rerr.Msg
	}
	return "E207", err.Error()
}

// emitDiagnostic reports the stable diagnostic on stderr and as Codex's
// native systemMessage, which is shown outside model context.
func emitDiagnostic(stdout, stderr io.Writer, code, msg string) {
	fmt.Fprintf(stderr, "learning-loop: codex-adapter: %s: %s\n", code, msg)
	env := map[string]any{"systemMessage": fmt.Sprintf("learning-loop: %s: %s", code, msg)}
	b, _ := json.Marshal(env)
	fmt.Fprintln(stdout, string(b))
}

// emitContext writes Codex's native additional-context envelope. An empty
// projection emits the envelope without additionalContext.
func emitContext(stdout io.Writer, context string) {
	hso := map[string]any{"hookEventName": "SessionStart"}
	if context != "" {
		hso["additionalContext"] = context
	}
	env := map[string]any{"hookSpecificOutput": hso}
	b, _ := json.Marshal(env)
	fmt.Fprintln(stdout, string(b))
}
