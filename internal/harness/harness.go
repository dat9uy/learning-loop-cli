package harness

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dat9uy/learning-loop-cli/internal/recordstore"
	"github.com/dat9uy/learning-loop-cli/internal/render"
)

// RuleName is the stable name of the standalone Concept Pressure Rule that
// every conformance case delivers.
const RuleName = "concept-pressure"

// Prompt is the user prompt sent to the Runtime under test. It must remain
// a separate message from the delivered Instruction.
const Prompt = "Reply with the single word done."

//go:embed concept-pressure.md
var ruleFixture []byte

// DefaultTimeout bounds one whole conformance case.
const DefaultTimeout = 2 * time.Minute

// Error is a stable, code-carrying failure.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Msg
}

// Installer is the production Installer surface the harness invokes. It is
// the same interface the Runtime Adapter packages expose; the harness never
// performs the Installer's changes itself.
type Installer interface {
	Install(projectRoot string) ([]string, error)
}

// Case is one real-Runtime conformance case. The harness owns shared
// fixture creation, Installer invocation, timeouts, semantic assertions,
// and diagnostic handling; each case owns its native launch and request
// decoding.
type Case interface {
	// Name is the Runtime name, e.g. "codex".
	Name() string
	// PinnedRuntime describes the exact pinned Runtime, e.g. "codex 0.147.0".
	PinnedRuntime() string
	// Installer returns the production Installer for this Runtime.
	Installer() Installer
	// Launch starts the pinned Runtime against the harness environment and
	// returns when it exits. It must launch the cached pinned executable
	// rather than whichever executable appears on PATH.
	Launch(ctx context.Context, env *Env) (LaunchResult, error)
	// DecodeRequest decodes one captured outbound request into the neutral
	// message shape the shared assertions consume.
	DecodeRequest(body []byte) (DecodedRequest, error)
}

// LaunchResult is the outcome of one Runtime launch.
type LaunchResult struct {
	Args     []string
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// DecodedRequest is the neutral shape of one outbound model request.
type DecodedRequest struct {
	Messages []Message
}

// Message is one message within a decoded request.
type Message struct {
	Role string
	Text string
}

// Options configures one conformance run.
type Options struct {
	// Keep retains the full temporary state for a standalone diagnostic
	// rerun. Successful runs clean temporary state unless Keep is set.
	Keep bool
	// Timeout bounds the whole case. Defaults to DefaultTimeout.
	Timeout time.Duration
	// RuntimeDir is the directory holding the pinned Runtime executable. It
	// is placed on PATH so the production Installer's version detection
	// resolves the pinned Runtime.
	RuntimeDir string
}

// Env is the isolated environment for one Runtime conformance case.
type Env struct {
	WorkDir     string
	Project     string
	RuntimeHome string
	SQLiteHome  string
	BinDir      string
	RuntimeDir  string
	Provider    *FakeProvider
	RuleBody    string
	Prompt      string
}

// Path returns the isolated PATH: the learning-loop bridge directory, the
// pinned Runtime directory, then the inherited PATH.
func (e *Env) Path() string {
	return e.BinDir + string(os.PathListSeparator) + e.RuntimeDir + string(os.PathListSeparator) + os.Getenv("PATH")
}

// Run executes one conformance case and returns the process exit code. It
// prints the PASS summary or the sanitized failure bundle to the given
// writers.
func Run(c Case, opts Options, stdout, stderr io.Writer) int {
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}
	env, err := Prepare(c, opts)
	if err != nil {
		fmt.Fprintf(stderr, "learning-loop: conformance %s: %v\n", c.Name(),
			&Error{Code: "E302", Msg: "preparing the disposable environment: " + err.Error()})
		return 1
	}
	if opts.Keep {
		fmt.Fprintf(stdout, "conformance %s: retaining full state at %s\n", c.Name(), env.WorkDir)
	} else {
		defer os.RemoveAll(env.WorkDir)
	}
	defer env.Provider.Close()

	originalPath := os.Getenv("PATH")
	os.Setenv("PATH", env.Path())
	defer os.Setenv("PATH", originalPath)
	installerMessages, installErr := c.Installer().Install(env.Project)
	if installErr != nil {
		return fail(c, env, opts, stderr, "installer", installerMessages, nil, installErr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()
	launch, launchErr := c.Launch(ctx, env)
	if launchErr != nil {
		return fail(c, env, opts, stderr, "launch", installerMessages, &launch, launchErr)
	}
	if launch.ExitCode != 0 {
		return fail(c, env, opts, stderr, "runtime exit", installerMessages, &launch,
			fmt.Errorf("Runtime exited with code %d", launch.ExitCode))
	}

	reqs := env.Provider.ResponsesRequests()
	if len(reqs) != 1 {
		return fail(c, env, opts, stderr, "requests", installerMessages, &launch,
			fmt.Errorf("expected exactly one outbound model request, got %d", len(reqs)))
	}
	decoded, err := c.DecodeRequest(reqs[0].Body)
	if err != nil {
		return fail(c, env, opts, stderr, "request decoding", installerMessages, &launch, err)
	}
	if err := AssertFirstRequest(decoded, env.RuleBody, env.Prompt); err != nil {
		return fail(c, env, opts, stderr, "first request", installerMessages, &launch, err)
	}

	fmt.Fprintf(stdout, "conformance %s: PASS\n", c.Name())
	fmt.Fprintf(stdout, "- pinned runtime: %s\n", c.PinnedRuntime())
	fmt.Fprintf(stdout, "- outbound model requests: 1\n")
	fmt.Fprintf(stdout, "- Rule delivered in the first request: %s\n", RuleName)
	for _, m := range installerMessages {
		fmt.Fprintf(stdout, "- installer: %s\n", m)
	}
	return 0
}

// Prepare creates the disposable Git project, the Record Store with the
// standalone Rule, the isolated Runtime environment, and the learning-loop
// PATH bridge. The caller owns cleanup of Env.WorkDir.
func Prepare(c Case, opts Options) (*Env, error) {
	workDir, err := os.MkdirTemp("", "learning-loop-conformance-*")
	if err != nil {
		return nil, err
	}
	env := &Env{WorkDir: workDir, RuntimeDir: opts.RuntimeDir, Prompt: Prompt}
	env.Project = filepath.Join(workDir, "project")
	if err := initGitProject(env.Project); err != nil {
		return nil, err
	}
	store := recordstore.New(env.Project)
	if err := store.Init(); err != nil {
		return nil, err
	}
	if _, err := store.CreateRevision(RuleName, ruleFixture); err != nil {
		return nil, err
	}
	expected, err := render.New().Render(env.Project)
	if err != nil {
		return nil, err
	}
	env.RuleBody = string(expected)
	env.RuntimeHome = filepath.Join(workDir, "runtime-home")
	env.SQLiteHome = filepath.Join(workDir, "sqlite-home")
	env.BinDir = filepath.Join(workDir, "bin")
	for _, dir := range []string{env.RuntimeHome, env.SQLiteHome, env.BinDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if err := os.Symlink(exe, filepath.Join(env.BinDir, "learning-loop")); err != nil {
		return nil, err
	}
	env.Provider = NewFakeProvider()
	if err := writeRuntimeConfig(env); err != nil {
		return nil, err
	}
	return env, nil
}

// writeRuntimeConfig writes the test-only Runtime Configuration the harness
// owns: the isolated Runtime home's config.toml. Codex gates project-local
// hooks on project trust, so the disposable project is marked trusted there.
func writeRuntimeConfig(env *Env) error {
	canonical, err := filepath.EvalSymlinks(env.Project)
	if err != nil {
		return err
	}
	config := fmt.Sprintf("[projects.%q]\ntrust_level = \"trusted\"\n", canonical)
	return os.WriteFile(filepath.Join(env.RuntimeHome, "config.toml"), []byte(config), 0o644)
}

// initGitProject creates a disposable Git project with one initial commit.
func initGitProject(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	run := func(args ...string) error {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %v: %v: %s", args, err, out)
		}
		return nil
	}
	if err := run("init", "-q", "-b", "main"); err != nil {
		return err
	}
	if err := run("config", "user.email", "conformance@learning-loop.invalid"); err != nil {
		return err
	}
	if err := run("config", "user.name", "learning-loop conformance"); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# conformance project\n"), 0o644); err != nil {
		return err
	}
	if err := run("add", "README.md"); err != nil {
		return err
	}
	if err := run("commit", "-q", "-m", "initial"); err != nil {
		return err
	}
	return nil
}

// AssertFirstRequest checks the semantic delivery contract on the first
// outbound model request: the current Rule body appears exactly once, Rule
// frontmatter is excluded, the user prompt stays a separate message, and no
// diagnostic contaminates the Instruction context. It never judges whether
// a model would follow the Rule.
func AssertFirstRequest(d DecodedRequest, ruleBody, prompt string) error {
	var instruction *Message
	count := 0
	for i := range d.Messages {
		m := &d.Messages[i]
		if strings.Contains(m.Text, ruleBody) {
			count++
			instruction = m
		}
	}
	if count != 1 {
		return fmt.Errorf("Rule body appears %d times in the first request, want exactly once", count)
	}
	for i := range d.Messages {
		m := &d.Messages[i]
		if strings.Contains(m.Text, "name: "+RuleName) || strings.Contains(m.Text, "description:") {
			return errors.New("Rule frontmatter leaked into the first request")
		}
	}
	if strings.Contains(instruction.Text, "learning-loop:") {
		return errors.New("diagnostic text leaked into the Instruction context")
	}
	if strings.Contains(instruction.Text, prompt) {
		return errors.New("user prompt merged into the Instruction message")
	}
	promptSeparate := false
	for i := range d.Messages {
		m := &d.Messages[i]
		if m != instruction && strings.Contains(m.Text, prompt) {
			promptSeparate = true
		}
	}
	if !promptSeparate {
		return errors.New("user prompt is not a separate message from the Instruction")
	}
	return nil
}

// fail prints the bounded, sanitized failure bundle and returns exit code 1.
func fail(c Case, env *Env, opts Options, stderr io.Writer, stage string, installerMessages []string, launch *LaunchResult, cause error) int {
	fmt.Fprintf(stderr, "conformance %s: FAIL\n", c.Name())
	fmt.Fprintf(stderr, "- stage: %s\n", stage)
	fmt.Fprintf(stderr, "- cause: %v\n", cause)
	fmt.Fprintf(stderr, "- pinned runtime: %s\n", c.PinnedRuntime())
	if len(installerMessages) > 0 {
		for _, m := range installerMessages {
			fmt.Fprintf(stderr, "- installer: %s\n", m)
		}
	}
	if launch != nil {
		fmt.Fprintf(stderr, "- launch arguments: %s\n", strings.Join(launch.Args, " "))
		fmt.Fprintf(stderr, "- runtime stdout: %s\n", bounded(string(launch.Stdout), 8*1024))
		fmt.Fprintf(stderr, "- runtime stderr: %s\n", bounded(string(launch.Stderr), 8*1024))
	}
	reqs := env.Provider.ResponsesRequests()
	fmt.Fprintf(stderr, "- outbound model requests: %d\n", len(reqs))
	for i, r := range reqs {
		fmt.Fprintf(stderr, "- request %d: %s %s\n", i+1, r.Method, r.Path)
		fmt.Fprintf(stderr, "  body: %s\n", bounded(string(r.Body), 4*1024))
	}
	if opts.Keep {
		fmt.Fprintf(stderr, "- full state retained at %s\n", env.WorkDir)
	}
	return 1
}

// bounded truncates s to at most n bytes with an explicit marker.
func bounded(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n... (truncated)"
}
