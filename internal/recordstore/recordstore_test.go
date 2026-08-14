package recordstore

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s := New(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return s
}

func TestInitCreatesRulesDirectory(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, DirName, RulesDirName))
	if err != nil {
		t.Fatalf("rules directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("rules path is not a directory")
	}
}

func TestInitIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	if err := s.Init(); err != nil {
		t.Fatalf("second Init: %v", err)
	}
}

func TestCreateRevisionNumbering(t *testing.T) {
	s := newTestStore(t)
	n, err := s.CreateRevision("alpha", []byte("body one"))
	if err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	if n != 1 {
		t.Fatalf("first revision number = %d, want 1", n)
	}
	n, err = s.CreateRevision("alpha", []byte("body two"))
	if err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	if n != 2 {
		t.Fatalf("second revision number = %d, want 2", n)
	}
	got, err := s.ReadRevision("alpha", 1)
	if err != nil {
		t.Fatalf("ReadRevision: %v", err)
	}
	if string(got) != "body one" {
		t.Fatalf("revision 1 = %q, want %q", got, "body one")
	}
	got, err = s.ReadRevision("alpha", 2)
	if err != nil {
		t.Fatalf("ReadRevision: %v", err)
	}
	if string(got) != "body two" {
		t.Fatalf("revision 2 = %q, want %q", got, "body two")
	}
}

func TestCreateRevisionUsesZeroPaddedNames(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateRevision("alpha", []byte("one")); err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	if _, err := s.CreateRevision("alpha", []byte("two")); err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	dir := filepath.Join(s.root, DirName, RulesDirName, "alpha")
	names, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("got %d files, want 2", len(names))
	}
	if names[0].Name() != "0001.md" || names[1].Name() != "0002.md" {
		t.Fatalf("file names = %q, %q; want 0001.md, 0002.md", names[0].Name(), names[1].Name())
	}
}

func TestCreateRevisionConflict(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateRevision("alpha", []byte("one")); err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	if _, err := s.CreateRevision("alpha", []byte("two")); err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	err := s.CreateRevisionAt("alpha", 2, []byte("overwrite attempt"))
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("CreateRevisionAt error = %v, want ConflictError", err)
	}
	if conflict.Rule != "alpha" || conflict.Revision != 2 {
		t.Fatalf("conflict = %+v, want rule alpha revision 2", conflict)
	}
	got, err := s.ReadRevision("alpha", 2)
	if err != nil {
		t.Fatalf("ReadRevision: %v", err)
	}
	if string(got) != "two" {
		t.Fatalf("revision 2 = %q, want %q (must not be overwritten)", got, "two")
	}
}

func TestCreateRevisionConcurrentClaimsOneSuccess(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateRevision("alpha", []byte("one")); err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	const workers = 20
	var wg sync.WaitGroup
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- s.CreateRevisionAt("alpha", 2, []byte("claimed"))
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	conflicts := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		var conflict *ConflictError
		if errors.As(err, &conflict) {
			conflicts++
			continue
		}
		t.Fatalf("unexpected error: %v", err)
	}
	if successes != 1 {
		t.Fatalf("successes = %d, want exactly 1", successes)
	}
	if conflicts != workers-1 {
		t.Fatalf("conflicts = %d, want %d", conflicts, workers-1)
	}
	got, err := s.ReadRevision("alpha", 2)
	if err != nil {
		t.Fatalf("ReadRevision: %v", err)
	}
	if string(got) != "claimed" {
		t.Fatalf("revision 2 = %q, want %q", got, "claimed")
	}
}

func TestRulesListsRuleDirectories(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateRevision("zebra", []byte("z")); err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	if _, err := s.CreateRevision("alpha", []byte("a")); err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	rules, err := s.Rules()
	if err != nil {
		t.Fatalf("Rules: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("rules = %v, want 2 entries", rules)
	}
	seen := map[string]bool{}
	for _, r := range rules {
		seen[r] = true
	}
	if !seen["alpha"] || !seen["zebra"] {
		t.Fatalf("rules = %v, want alpha and zebra", rules)
	}
}

func TestRulesErrorsWhenNotInitialized(t *testing.T) {
	s := New(t.TempDir())
	_, err := s.Rules()
	if !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("Rules error = %v, want ErrNotInitialized", err)
	}
}

func TestRevisionsAscending(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateRevision("alpha", []byte("one")); err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	if _, err := s.CreateRevision("alpha", []byte("two")); err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	revs, err := s.Revisions("alpha")
	if err != nil {
		t.Fatalf("Revisions: %v", err)
	}
	if len(revs) != 2 || revs[0] != 1 || revs[1] != 2 {
		t.Fatalf("revisions = %v, want [1 2]", revs)
	}
}

func TestRevisionsEmptyForUnknownRule(t *testing.T) {
	s := newTestStore(t)
	revs, err := s.Revisions("ghost")
	if err != nil {
		t.Fatalf("Revisions: %v", err)
	}
	if len(revs) != 0 {
		t.Fatalf("revisions = %v, want empty", revs)
	}
}
