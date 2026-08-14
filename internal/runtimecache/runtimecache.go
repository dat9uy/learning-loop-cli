// Package runtimecache manages the development Runtime cache: exact pinned
// Runtime executables downloaded once by an explicit setup command and
// consumed by the conformance commands. Conformance never downloads or
// installs a Runtime; it reports the exact setup remediation when a cached
// prerequisite is absent or invalid.
package runtimecache

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// CodexVersion is the pinned Codex CLI version whose session-start hook
// contract the conformance case validates.
const CodexVersion = "0.147.0"

// codexReleaseTag is the GitHub release tag carrying the pinned Codex CLI.
const codexReleaseTag = "rust-v0.147.0"

// Error is a stable, code-carrying failure.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Msg
}

// platform describes one supported download target.
type platform struct {
	archive string
	sha256  string
	binary  string
}

// codexPlatforms maps GOOS/GOARCH to the pinned Codex package archive. The
// checksums are the values published in the release's codex-package_SHA256SUMS.
var codexPlatforms = map[string]platform{
	"linux/amd64": {
		archive: "codex-package-x86_64-unknown-linux-musl.tar.gz",
		sha256:  "bd758d53d56e41dc65e045f4589df79a038ed197a011adcb52a258e6ad64cfda",
		binary:  "bin/codex",
	},
	"linux/arm64": {
		archive: "codex-package-aarch64-unknown-linux-musl.tar.gz",
		sha256:  "89cbf79bd5ae6f9c58da47e8079f311c84219350c9c43c070d42f3e9b2a81401",
		binary:  "bin/codex",
	},
	"darwin/amd64": {
		archive: "codex-package-x86_64-apple-darwin.tar.gz",
		sha256:  "d91e59133daf923bc45d76e3da4af8ae9ef62a0231da18488da0cd573b6e9d63",
		binary:  "bin/codex",
	},
	"darwin/arm64": {
		archive: "codex-package-aarch64-apple-darwin.tar.gz",
		sha256:  "17b2984eb22b607e3d0c25728252fc90f510e476bad39a6d9f45cdb1aa685432",
		binary:  "bin/codex",
	},
	"windows/amd64": {
		archive: "codex-package-x86_64-pc-windows-msvc.tar.gz",
		sha256:  "c156c8feb8cb20197bf74d2c6daffed1fec0a8c21a03bc2ca90d7ff81927b0c5",
		binary:  "bin/codex.exe",
	},
	"windows/arm64": {
		archive: "codex-package-aarch64-pc-windows-msvc.tar.gz",
		sha256:  "4533928d72ac4d7c19f16e8c4acdfd02dc255d2aeeb2f6d7dfd45493ec4c0806",
		binary:  "bin/codex.exe",
	},
}

// CacheDir returns the development Runtime cache directory:
// $LEARNING_LOOP_CACHE, else $XDG_CACHE_HOME/learning-loop/runtimes, else
// ~/.cache/learning-loop/runtimes.
func CacheDir() (string, error) {
	if d := os.Getenv("LEARNING_LOOP_CACHE"); d != "" {
		return d, nil
	}
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, "learning-loop", "runtimes"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", &Error{Code: "E306", Msg: fmt.Sprintf("cannot resolve the home directory: %v", err)}
	}
	return filepath.Join(home, ".cache", "learning-loop", "runtimes"), nil
}

// CodexBinaryPath returns the path of the cached pinned Codex executable,
// validating that it exists, is executable, and matches the recorded
// checksum. It never downloads or installs. When the cached prerequisite is
// absent or invalid it returns an *Error with the exact setup remediation.
func CodexBinaryPath() (string, error) {
	dir, err := CacheDir()
	if err != nil {
		return "", err
	}
	p, err := currentPlatform()
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "codex-"+CodexVersion, filepath.Base(p.binary))
	if err := validateBinary(bin); err != nil {
		return "", &Error{Code: "E300", Msg: fmt.Sprintf("cached Codex %s is absent or invalid (%v); run `learning-loop runtime-setup codex`", CodexVersion, err)}
	}
	return bin, nil
}

// SetupCodex downloads the pinned Codex CLI, verifies its published
// checksum, and stores it in the development Runtime cache. It is
// idempotent: an already valid cached binary is left untouched.
func SetupCodex() error {
	return setupCodex(deps{
		platform: currentPlatform,
		download: downloadArchive,
	})
}

type deps struct {
	platform func() (platform, error)
	download func(url string) ([]byte, error)
}

func setupCodex(d deps) error {
	p, err := d.platform()
	if err != nil {
		return err
	}
	dir, err := CacheDir()
	if err != nil {
		return err
	}
	target := filepath.Join(dir, "codex-"+CodexVersion)
	bin := filepath.Join(target, filepath.Base(p.binary))
	if err := validateBinary(bin); err == nil {
		return nil
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("creating the Runtime cache: %v", err)}
	}
	url := "https://github.com/openai/codex/releases/download/" + codexReleaseTag + "/" + p.archive
	archive, err := d.download(url)
	if err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("downloading %s: %v", url, err)}
	}
	if sum := sha256.Sum256(archive); hex.EncodeToString(sum[:]) != p.sha256 {
		return &Error{Code: "E301", Msg: fmt.Sprintf("checksum mismatch for %s; refusing to cache it", p.archive)}
	}
	content, err := extractEntry(archive, p.binary)
	if err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("extracting %s from %s: %v", p.binary, p.archive, err)}
	}
	if err := writeBinary(target, bin, content); err != nil {
		return err
	}
	return nil
}

// currentPlatform resolves the download target for the running platform.
func currentPlatform() (platform, error) {
	p, ok := codexPlatforms[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok {
		return platform{}, &Error{Code: "E306", Msg: fmt.Sprintf("no pinned Codex %s archive for %s/%s", CodexVersion, runtime.GOOS, runtime.GOARCH)}
	}
	return p, nil
}

// downloadArchive fetches url with a generous timeout.
func downloadArchive(url string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// extractEntry returns the bytes of the named entry inside a tar.gz archive.
func extractEntry(archive []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("entry %q not found", name)
		}
		if err != nil {
			return nil, err
		}
		if hdr.Name == name {
			return io.ReadAll(tr)
		}
	}
}

// writeBinary writes the binary and its recorded checksum atomically.
func writeBinary(target, bin string, content []byte) error {
	tmp, err := os.CreateTemp(target, ".codex-*")
	if err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("writing the cached binary: %v", err)}
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return &Error{Code: "E301", Msg: fmt.Sprintf("writing the cached binary: %v", err)}
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return &Error{Code: "E301", Msg: fmt.Sprintf("writing the cached binary: %v", err)}
	}
	if err := tmp.Close(); err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("writing the cached binary: %v", err)}
	}
	if err := os.Rename(tmpName, bin); err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("writing the cached binary: %v", err)}
	}
	sum := sha256.Sum256(content)
	record := hex.EncodeToString(sum[:]) + "\n"
	sumTmp, err := os.CreateTemp(target, ".codex-*")
	if err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("recording the cached checksum: %v", err)}
	}
	sumTmpName := sumTmp.Name()
	defer os.Remove(sumTmpName)
	if _, err := sumTmp.Write([]byte(record)); err != nil {
		sumTmp.Close()
		return &Error{Code: "E301", Msg: fmt.Sprintf("recording the cached checksum: %v", err)}
	}
	if err := sumTmp.Close(); err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("recording the cached checksum: %v", err)}
	}
	if err := os.Rename(sumTmpName, bin+".sha256"); err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("recording the cached checksum: %v", err)}
	}
	return nil
}

// validateBinary checks that the cached binary exists, is executable, and
// matches its recorded checksum.
func validateBinary(bin string) error {
	info, err := os.Stat(bin)
	if err != nil {
		return err
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("not executable")
	}
	recorded, err := os.ReadFile(bin + ".sha256")
	if err != nil {
		return fmt.Errorf("missing recorded checksum")
	}
	content, err := os.ReadFile(bin)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(content)
	if hex.EncodeToString(sum[:]) != strings.TrimSpace(string(recorded)) {
		return fmt.Errorf("checksum mismatch")
	}
	return nil
}
