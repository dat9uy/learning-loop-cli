package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dat9uy/learning-loop-cli/internal/recordstore"
)

// stubCase is a minimal Case for exercising the shared harness.
type stubCase struct{}

func (stubCase) Name() string          { return "stub" }
func (stubCase) PinnedRuntime() string { return "stub 0.0.0" }
func (stubCase) Installer() Installer  { return nil }
func (stubCase) Launch(context.Context, *Env) (LaunchResult, error) {
	return LaunchResult{ExitCode: 0}, nil
}
func (stubCase) DecodeRequest(body []byte) (DecodedRequest, error) {
	return DecodedRequest{}, nil
}

func TestEmbeddedFixtureMatchesRepoFixture(t *testing.T) {
	repo, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "concept-pressure.md"))
	if err != nil {
		t.Fatalf("reading repo fixture: %v", err)
	}
	if string(ruleFixture) != string(repo) {
		t.Fatalf("embedded harness fixture drifted from fixtures/concept-pressure.md")
	}
}

func TestPrepareCreatesDisposableEnvironment(t *testing.T) {
	env, err := Prepare(stubCase{}, Options{RuntimeDir: "/nonexistent"})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	defer os.RemoveAll(env.WorkDir)

	if _, err := os.Stat(filepath.Join(env.Project, ".git")); err != nil {
		t.Fatalf("disposable Git project missing: %v", err)
	}
	store := recordstore.New(env.Project)
	rules, err := store.Rules()
	if err != nil {
		t.Fatalf("Rules: %v", err)
	}
	if len(rules) != 1 || rules[0] != RuleName {
		t.Fatalf("rules = %v, want [%s]", rules, RuleName)
	}
	revs, err := store.Revisions(RuleName)
	if err != nil || len(revs) != 1 {
		t.Fatalf("revisions = %v, %v; want one current revision", revs, err)
	}
	if !strings.Contains(env.RuleBody, "Proposals that add or change durable concepts must") {
		t.Fatalf("RuleBody = %q, want the Concept Pressure body", env.RuleBody)
	}
	if strings.Contains(env.RuleBody, "name: concept-pressure") {
		t.Fatalf("RuleBody contains frontmatter")
	}
	if _, err := os.Stat(filepath.Join(env.BinDir, "learning-loop")); err != nil {
		t.Fatalf("learning-loop PATH bridge missing: %v", err)
	}
	if !strings.Contains(env.Path(), env.BinDir) || !strings.Contains(env.Path(), "/nonexistent") {
		t.Fatalf("Path = %q, want BinDir and RuntimeDir", env.Path())
	}
	config, err := os.ReadFile(filepath.Join(env.RuntimeHome, "config.toml"))
	if err != nil {
		t.Fatalf("runtime config.toml: %v", err)
	}
	if !strings.Contains(string(config), "trust_level = \"trusted\"") {
		t.Fatalf("config.toml = %q, want the disposable project marked trusted", config)
	}
}

func TestAssertFirstRequestPasses(t *testing.T) {
	d := DecodedRequest{Messages: []Message{
		{Role: "developer", Text: "Proposals that add or change durable concepts must:\n\n- name the preserved invariant,\n"},
		{Role: "user", Text: "Reply with the single word done."},
	}}
	if err := AssertFirstRequest(d, "Proposals that add or change durable concepts must:", Prompt); err != nil {
		t.Fatalf("AssertFirstRequest: %v", err)
	}
}

func TestAssertFirstRequestFailures(t *testing.T) {
	body := "Proposals that add or change durable concepts must:"
	cases := []struct {
		name string
		d    DecodedRequest
		want string
	}{
		{
			name: "rule body absent",
			d:    DecodedRequest{Messages: []Message{{Role: "user", Text: Prompt}}},
			want: "appears 0 times",
		},
		{
			name: "rule body twice",
			d: DecodedRequest{Messages: []Message{
				{Role: "developer", Text: body},
				{Role: "developer", Text: body},
				{Role: "user", Text: Prompt},
			}},
			want: "appears 2 times",
		},
		{
			name: "frontmatter leaked",
			d: DecodedRequest{Messages: []Message{
				{Role: "developer", Text: "name: concept-pressure\n" + body},
				{Role: "user", Text: Prompt},
			}},
			want: "frontmatter",
		},
		{
			name: "diagnostic leaked",
			d: DecodedRequest{Messages: []Message{
				{Role: "developer", Text: "learning-loop: E205: reading the Codex event\n" + body},
				{Role: "user", Text: Prompt},
			}},
			want: "diagnostic",
		},
		{
			name: "prompt merged into instruction",
			d: DecodedRequest{Messages: []Message{
				{Role: "developer", Text: body + "\n" + Prompt},
			}},
			want: "merged into the Instruction message",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := AssertFirstRequest(tc.d, body, Prompt)
			if err == nil {
				t.Fatalf("AssertFirstRequest succeeded, want failure containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestBoundedTruncates(t *testing.T) {
	if got := bounded("short", 8*1024); got != "short" {
		t.Fatalf("bounded short = %q", got)
	}
	got := bounded("0123456789", 5)
	if !strings.HasPrefix(got, "01234") || !strings.Contains(got, "truncated") {
		t.Fatalf("bounded long = %q", got)
	}
}
