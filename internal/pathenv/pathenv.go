// Package pathenv resolves executables against an explicit PATH and creates a
// child environment with that PATH, without mutating the parent process.
package pathenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// LookPath is the explicit-PATH counterpart to exec.LookPath.
func LookPath(path, name string) (string, error) {
	names := []string{name}
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		for _, ext := range strings.Split(os.Getenv("PATHEXT"), ";") {
			if ext != "" {
				names = append(names, name+ext)
			}
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			dir = "."
		}
		for _, candidateName := range names {
			candidate := filepath.Join(dir, candidateName)
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() && (runtime.GOOS == "windows" || info.Mode()&0o111 != 0) {
				return candidate, nil
			}
		}
	}
	return "", exec.ErrNotFound
}

// WithPath returns a copy of env with its existing PATH replaced.
func WithPath(env []string, path string) []string {
	env = append([]string(nil), env...)
	for i, value := range env {
		if strings.HasPrefix(value, "PATH=") {
			env[i] = "PATH=" + path
			return env
		}
	}
	return append(env, "PATH="+path)
}
