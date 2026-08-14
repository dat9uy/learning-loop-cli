// Package recordstore implements the append-only memory of learning-loop
// records. Each Rule identity owns a directory of zero-padded, monotonically
// numbered, immutable Markdown revisions.
package recordstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	// DirName is the project-local directory holding the Record Store.
	DirName = ".learning-loop"
	// RulesDirName is the directory under DirName holding one directory per Rule.
	RulesDirName = "rules"
)

// ErrNotInitialized reports that the project has no Record Store.
var ErrNotInitialized = errors.New("record store not initialized")

var revisionName = regexp.MustCompile(`^(\d+)\.md$`)

// ConflictError reports that a revision number is already claimed.
type ConflictError struct {
	Rule     string
	Revision int
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("revision %04d for rule %q already exists", e.Revision, e.Rule)
}

// Store is the on-disk Record Store for one project root.
type Store struct {
	root string
}

// New returns a Store rooted at the given project root.
func New(root string) *Store {
	return &Store{root: root}
}

// Init creates the Record Store directory structure. It is idempotent.
func (s *Store) Init() error {
	return os.MkdirAll(filepath.Join(s.root, DirName, RulesDirName), 0o755)
}

// CreateRevision appends the next revision of rule with the given content.
// It never overwrites an existing revision: a concurrent or repeated claim of
// the same next revision returns a *ConflictError.
func (s *Store) CreateRevision(rule string, content []byte) (int, error) {
	if err := validateRuleName(rule); err != nil {
		return 0, err
	}
	revs, err := s.Revisions(rule)
	if err != nil {
		return 0, err
	}
	next := 1
	if len(revs) > 0 {
		next = revs[len(revs)-1] + 1
	}
	if err := s.CreateRevisionAt(rule, next, content); err != nil {
		return 0, err
	}
	return next, nil
}

// CreateRevisionAt claims exactly revision n of rule. The claim is atomic:
// concurrent attempts to claim the same revision produce one success and
// *ConflictError for every other attempt. An existing revision is never
// overwritten.
func (s *Store) CreateRevisionAt(rule string, n int, content []byte) error {
	if err := validateRuleName(rule); err != nil {
		return err
	}
	ruleDir := filepath.Join(s.root, DirName, RulesDirName, rule)
	if err := os.MkdirAll(ruleDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(ruleDir, revisionFileName(n))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if os.IsExist(err) {
		return &ConflictError{Rule: rule, Revision: n}
	}
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Rules returns the stable Rule names present in the Record Store.
func (s *Store) Rules() ([]string, error) {
	root := filepath.Join(s.root, DirName, RulesDirName)
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, ErrNotInitialized
	}
	if err != nil {
		return nil, err
	}
	var rules []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			rules = append(rules, e.Name())
		}
	}
	return rules, nil
}

// Revisions returns the revision numbers present for rule, ascending. Files
// that do not match the zero-padded revision naming pattern are not
// revisions and are ignored.
func (s *Store) Revisions(rule string) ([]int, error) {
	ruleDir := filepath.Join(s.root, DirName, RulesDirName, rule)
	entries, err := os.ReadDir(ruleDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var revs []int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := revisionName.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		revs = append(revs, n)
	}
	sort.Ints(revs)
	return revs, nil
}

// ReadRevision returns the raw bytes of revision n of rule.
func (s *Store) ReadRevision(rule string, n int) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.root, DirName, RulesDirName, rule, revisionFileName(n)))
}

func revisionFileName(n int) string {
	return fmt.Sprintf("%04d.md", n)
}

func validateRuleName(rule string) error {
	if rule == "" {
		return errors.New("rule name must not be empty")
	}
	if rule == "." || rule == ".." {
		return fmt.Errorf("invalid rule name %q", rule)
	}
	if strings.HasPrefix(rule, ".") {
		return fmt.Errorf("invalid rule name %q", rule)
	}
	if strings.ContainsAny(rule, `/\`) {
		return fmt.Errorf("invalid rule name %q", rule)
	}
	return nil
}
