// Package cli implements the learning-loop command-line interface.
package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/dat9uy/learning-loop-cli/internal/recordstore"
	"github.com/dat9uy/learning-loop-cli/internal/render"
)

const usage = `learning-loop — deliver standalone Rules as Instructions

Usage:
  learning-loop init <project-root>     initialize the project's Record Store
  learning-loop render <project-root>    render current Rules as Instruction Markdown
`

// Run executes the CLI and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usageError(stderr)
		return 2
	}
	switch args[0] {
	case "init":
		if len(args) != 2 {
			usageError(stderr)
			return 2
		}
		return runInit(args[1], stdout, stderr)
	case "render":
		if len(args) != 2 {
			usageError(stderr)
			return 2
		}
		return runRender(args[1], stdout, stderr)
	default:
		usageError(stderr)
		return 2
	}
}

func usageError(stderr io.Writer) {
	fmt.Fprint(stderr, usage)
}

func runInit(root string, stdout, stderr io.Writer) int {
	if err := render.ValidateProjectRoot(root); err != nil {
		fmt.Fprintf(stderr, "learning-loop: init: %v\n", err)
		return 1
	}
	store := recordstore.New(root)
	if err := store.Init(); err != nil {
		fmt.Fprintf(stderr, "learning-loop: init: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "initialized Record Store at %s\n", filepath.Join(root, recordstore.DirName, recordstore.RulesDirName))
	return 0
}

func runRender(root string, stdout, stderr io.Writer) int {
	out, err := render.New().Render(root)
	if err != nil {
		fmt.Fprintf(stderr, "learning-loop: render: %v\n", err)
		return 1
	}
	if _, err := stdout.Write(out); err != nil {
		fmt.Fprintf(stderr, "learning-loop: render: %v\n", err)
		return 1
	}
	return 0
}
