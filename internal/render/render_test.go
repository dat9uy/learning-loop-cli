package render

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dat9uy/learning-loop-cli/internal/recordstore"
)

func newProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := recordstore.New(root).Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return root
}

func addRevision(t *testing.T, root, rule, content string) {
	t.Helper()
	if _, err := recordstore.New(root).CreateRevision(rule, []byte(content)); err != nil {
		t.Fatalf("CreateRevision(%q): %v", rule, err)
	}
}

func render(t *testing.T, root string) (string, error) {
	t.Helper()
	out, err := New().Render(root)
	return string(out), err
}

func TestRenderRequiresAbsoluteRoot(t *testing.T) {
	_, err := render(t, "relative/project")
	var rerr *Error
	if !errors.As(err, &rerr) {
		t.Fatalf("Render error = %v, want *Error", err)
	}
	if rerr.Code != "E100" {
		t.Fatalf("error code = %q, want E100", rerr.Code)
	}
}

func TestRenderRequiresExistingDirectory(t *testing.T) {
	_, err := render(t, filepath.Join(t.TempDir(), "missing"))
	var rerr *Error
	if !errors.As(err, &rerr) {
		t.Fatalf("Render error = %v, want *Error", err)
	}
	if rerr.Code != "E100" {
		t.Fatalf("error code = %q, want E100", rerr.Code)
	}
}

func TestRenderErrorsWhenNotInitialized(t *testing.T) {
	_, err := render(t, t.TempDir())
	var rerr *Error
	if !errors.As(err, &rerr) {
		t.Fatalf("Render error = %v, want *Error", err)
	}
	if rerr.Code != "E101" {
		t.Fatalf("error code = %q, want E101", rerr.Code)
	}
}

func TestRenderStripsFrontmatter(t *testing.T) {
	root := newProject(t)
	addRevision(t, root, "alpha", "---\nname: alpha\ndescription: d\n---\nbody line one\nbody line two\n")
	got, err := render(t, root)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "body line one\nbody line two\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if strings.Contains(got, "name:") || strings.Contains(got, "description:") {
		t.Fatalf("output contains frontmatter: %q", got)
	}
}

func TestRenderSelectsHighestRevision(t *testing.T) {
	root := newProject(t)
	addRevision(t, root, "alpha", "---\nname: alpha\n---\nversion one\n")
	addRevision(t, root, "alpha", "---\nname: alpha\n---\nversion two\n")
	got, err := render(t, root)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got != "version two\n" {
		t.Fatalf("output = %q, want %q", got, "version two\n")
	}
}

func TestRenderSortsRulesByName(t *testing.T) {
	root := newProject(t)
	addRevision(t, root, "zebra", "---\nname: zebra\n---\nzebra body\n")
	addRevision(t, root, "alpha", "---\nname: alpha\n---\nalpha body\n")
	addRevision(t, root, "mike", "---\nname: mike\n---\nmike body\n")
	got, err := render(t, root)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "alpha body\n\nmike body\n\nzebra body\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRenderJoinsWithExactlyOneBlankLine(t *testing.T) {
	root := newProject(t)
	addRevision(t, root, "alpha", "---\nname: alpha\n---\nfirst\n")
	addRevision(t, root, "beta", "---\nname: beta\n---\nsecond\n")
	got, err := render(t, root)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "first\n\nsecond\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if strings.Contains(got, "\n\n\n") {
		t.Fatalf("output contains more than one blank line: %q", got)
	}
}

func TestRenderPreservesBodyBytes(t *testing.T) {
	root := newProject(t)
	body := "  indented line\n\n\npara with  trailing spaces  \n\ttabbed\n"
	addRevision(t, root, "alpha", "---\nname: alpha\n---\n"+body)
	got, err := render(t, root)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "  indented line\n\n\npara with  trailing spaces  \n\ttabbed\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRenderFailsOnMalformedRevision(t *testing.T) {
	root := newProject(t)
	addRevision(t, root, "alpha", "no frontmatter here\n")
	_, err := render(t, root)
	var rerr *Error
	if !errors.As(err, &rerr) {
		t.Fatalf("Render error = %v, want *Error", err)
	}
	if rerr.Code != "E103" {
		t.Fatalf("error code = %q, want E103", rerr.Code)
	}
}

func TestRenderFailsOnInconsistentIdentity(t *testing.T) {
	root := newProject(t)
	addRevision(t, root, "alpha", "---\nname: other\n---\nbody\n")
	_, err := render(t, root)
	var rerr *Error
	if !errors.As(err, &rerr) {
		t.Fatalf("Render error = %v, want *Error", err)
	}
	if rerr.Code != "E104" {
		t.Fatalf("error code = %q, want E104", rerr.Code)
	}
}

func TestRenderFailsOnRuleWithoutRevisions(t *testing.T) {
	root := newProject(t)
	addRevision(t, root, "alpha", "---\nname: alpha\n---\nbody\n")
	if err := os.MkdirAll(filepath.Join(root, recordstore.DirName, recordstore.RulesDirName, "empty-rule"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	_, err := render(t, root)
	var rerr *Error
	if !errors.As(err, &rerr) {
		t.Fatalf("Render error = %v, want *Error", err)
	}
	if rerr.Code != "E102" {
		t.Fatalf("error code = %q, want E102", rerr.Code)
	}
}

func TestRenderFailsOnUnavailableRevision(t *testing.T) {
	root := newProject(t)
	ruleDir := filepath.Join(root, recordstore.DirName, recordstore.RulesDirName, "alpha")
	if err := os.MkdirAll(ruleDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink("nowhere", filepath.Join(ruleDir, "0001.md")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	_, err := render(t, root)
	var rerr *Error
	if !errors.As(err, &rerr) {
		t.Fatalf("Render error = %v, want *Error", err)
	}
	if rerr.Code != "E105" {
		t.Fatalf("error code = %q, want E105", rerr.Code)
	}
}

func TestRenderEmptyStoreProducesEmptyOutput(t *testing.T) {
	root := newProject(t)
	got, err := render(t, root)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got != "" {
		t.Fatalf("output = %q, want empty", got)
	}
}

func TestRenderPreservesFinalBodyTrailingNewlines(t *testing.T) {
	root := newProject(t)
	addRevision(t, root, "alpha", "---\nname: alpha\n---\nbody\n\n")
	got, err := render(t, root)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got != "body\n\n" {
		t.Fatalf("output = %q, want %q", got, "body\n\n")
	}
}

func TestRenderIgnoresNonRevisionFiles(t *testing.T) {
	root := newProject(t)
	addRevision(t, root, "alpha", "---\nname: alpha\n---\nbody\n")
	ruleDir := filepath.Join(root, recordstore.DirName, recordstore.RulesDirName, "alpha")
	if err := os.WriteFile(filepath.Join(ruleDir, "notes.txt"), []byte("not a revision"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := render(t, root)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got != "body\n" {
		t.Fatalf("output = %q, want %q", got, "body\n")
	}
}

func TestRenderIsAllOrNothing(t *testing.T) {
	root := newProject(t)
	addRevision(t, root, "good", "---\nname: good\n---\ngood body\n")
	addRevision(t, root, "bad", "---\nname: bad\n---\n")
	_, err := render(t, root)
	var rerr *Error
	if !errors.As(err, &rerr) {
		t.Fatalf("Render error = %v, want *Error", err)
	}
	if rerr.Code != "E103" {
		t.Fatalf("error code = %q, want E103", rerr.Code)
	}
}

func TestRenderConceptPressureFixture(t *testing.T) {
	root := newProject(t)
	fixture, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "concept-pressure.md"))
	if err != nil {
		t.Fatalf("ReadFile fixture: %v", err)
	}
	if _, err := recordstore.New(root).CreateRevision("concept-pressure", fixture); err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	got, err := render(t, root)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, phrase := range []string{
		"name the preserved invariant",
		"account for permanent propagation cost",
		"identify the deterministic consumer for every proposed value",
		"choose the lowest durable Abstraction Level that preserves the invariant",
		"keep the concept surface small and prevent agent-visible distinctions from being mistaken for deterministic behavior",
	} {
		if !strings.Contains(got, phrase) {
			t.Fatalf("output missing %q:\n%s", phrase, got)
		}
	}
	if strings.Contains(got, "description:") {
		t.Fatalf("output contains frontmatter: %q", got)
	}
}
