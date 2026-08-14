// Package render is the rendering module: it discovers the selected project's
// Record Store, selects each Rule's current revision, parses and validates
// records, and concatenates the bodies into deterministic Instruction
// Markdown. All production callers use the Renderer interface.
package render

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/dat9uy/learning-loop-cli/internal/recordstore"
	"gopkg.in/yaml.v3"
)

// Renderer renders every current Rule in a project as Instruction Markdown.
type Renderer interface {
	// Render returns the Instruction Markdown for every current Rule in the
	// project rooted at projectRoot, which must be an absolute path. It
	// returns an *Error on any invalid or unavailable input and never
	// returns partial output.
	Render(projectRoot string) ([]byte, error)
}

// Error is a stable, code-carrying rendering failure.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Msg
}

// ValidateProjectRoot checks that projectRoot is an absolute path to an
// existing directory. It returns an *Error with code E100 otherwise.
func ValidateProjectRoot(projectRoot string) error {
	if !filepath.IsAbs(projectRoot) {
		return &Error{Code: "E100", Msg: fmt.Sprintf("project root must be an absolute path: %q", projectRoot)}
	}
	info, err := os.Stat(projectRoot)
	if err != nil || !info.IsDir() {
		return &Error{Code: "E100", Msg: fmt.Sprintf("project root is not a directory: %q", projectRoot)}
	}
	return nil
}

type renderer struct{}

// New returns a Renderer.
func New() Renderer {
	return &renderer{}
}

func (r *renderer) Render(projectRoot string) ([]byte, error) {
	if err := ValidateProjectRoot(projectRoot); err != nil {
		return nil, err
	}
	store := recordstore.New(projectRoot)
	rules, err := store.Rules()
	if errors.Is(err, recordstore.ErrNotInitialized) {
		return nil, &Error{Code: "E101", Msg: fmt.Sprintf("no Record Store at %q; run learning-loop init", projectRoot)}
	}
	if err != nil {
		return nil, &Error{Code: "E105", Msg: fmt.Sprintf("reading Record Store: %v", err)}
	}
	sort.Strings(rules)
	bodies := make([][]byte, 0, len(rules))
	for _, rule := range rules {
		body, err := r.currentBody(store, rule)
		if err != nil {
			return nil, err
		}
		bodies = append(bodies, body)
	}
	return join(bodies), nil
}

func (r *renderer) currentBody(store *recordstore.Store, rule string) ([]byte, error) {
	revs, err := store.Revisions(rule)
	if err != nil {
		return nil, &Error{Code: "E105", Msg: fmt.Sprintf("rule %q: %v", rule, err)}
	}
	if len(revs) == 0 {
		return nil, &Error{Code: "E102", Msg: fmt.Sprintf("rule %q has no current revision", rule)}
	}
	n := revs[len(revs)-1]
	content, err := store.ReadRevision(rule, n)
	if err != nil {
		return nil, &Error{Code: "E105", Msg: fmt.Sprintf("rule %q revision %04d: %v", rule, n, err)}
	}
	body, name, err := parseRevision(content)
	if err != nil {
		return nil, &Error{Code: "E103", Msg: fmt.Sprintf("rule %q revision %04d: %v", rule, n, err)}
	}
	if name != rule {
		return nil, &Error{Code: "E104", Msg: fmt.Sprintf("rule %q revision %04d: frontmatter name %q does not match the rule directory", rule, n, name)}
	}
	return body, nil
}

// parseRevision splits a revision into its Markdown body and frontmatter
// name. The body is returned byte-for-byte apart from nothing: it is the raw
// bytes after the closing frontmatter delimiter.
func parseRevision(content []byte) (body []byte, name string, err error) {
	const open = "---\n"
	const close = "\n---\n"
	if !bytes.HasPrefix(content, []byte(open)) {
		return nil, "", errors.New("missing frontmatter")
	}
	rest := content[len(open):]
	end := bytes.Index(rest, []byte(close))
	if end < 0 {
		return nil, "", errors.New("unterminated frontmatter")
	}
	var meta struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal(rest[:end], &meta); err != nil {
		return nil, "", fmt.Errorf("malformed frontmatter: %v", err)
	}
	if meta.Name == "" {
		return nil, "", errors.New("frontmatter missing name")
	}
	body = rest[end+len(close):]
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, "", errors.New("empty body")
	}
	return body, meta.Name, nil
}

// join concatenates bodies in order with exactly one blank line between
// them. Every body except the last is trimmed of trailing newlines for the
// join; the final body's trailing newlines are preserved byte-for-byte.
func join(bodies [][]byte) []byte {
	var out []byte
	for i, b := range bodies {
		if i > 0 {
			out = append(out, '\n', '\n')
		}
		out = append(out, bytes.TrimRight(b, "\n")...)
	}
	if n := len(bodies); n > 0 {
		out = append(out, trailingNewlines(bodies[n-1])...)
	}
	return out
}

func trailingNewlines(b []byte) []byte {
	return b[len(bytes.TrimRight(b, "\n")):]
}
