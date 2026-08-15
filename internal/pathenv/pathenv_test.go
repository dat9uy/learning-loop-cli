package pathenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLookPathUsesOnlyProvidedPath(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := LookPath(dir, "tool")
	if err != nil || got != tool {
		t.Fatalf("LookPath = %q, %v; want %q", got, err, tool)
	}
	if _, err := LookPath(t.TempDir(), "tool"); err == nil {
		t.Fatalf("LookPath found a tool outside the provided PATH")
	}
}

func TestWithPathReplacesExistingPathWithoutMutatingInput(t *testing.T) {
	input := []string{"HOME=/tmp", "PATH=/old", "LANG=C"}
	got := WithPath(input, "/new")
	if strings.Join(input, "\x00") != "HOME=/tmp\x00PATH=/old\x00LANG=C" {
		t.Fatalf("WithPath mutated input: %v", input)
	}
	if strings.Join(got, "\x00") != "HOME=/tmp\x00PATH=/new\x00LANG=C" {
		t.Fatalf("WithPath = %v", got)
	}
}
