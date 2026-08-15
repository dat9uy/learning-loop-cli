// Package runtimecache manages the development Runtime cache: exact pinned
// Runtime executables downloaded once by an explicit setup command and
// consumed by the conformance commands. Conformance never downloads or
// installs a Runtime; it reports the exact setup remediation when a cached
// prerequisite is absent or invalid.
package runtimecache

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// CodexVersion is the pinned Codex CLI version whose session-start hook
// contract the conformance case validates.
const CodexVersion = "0.147.0"

// OpenCodeVersion is the pinned OpenCode version whose plugin contract the
// conformance case validates.
const OpenCodeVersion = "1.18.18"

// PiVersion is the pinned pi version whose extension contract the
// conformance case validates.
const PiVersion = "0.84.1"

// PiEntryPoint is the pi CLI entry point inside the cached npm tree,
// relative to the tree root.
const PiEntryPoint = "package/dist/cli.js"

// piTarballName returns the npm registry tarball file name of the pinned pi
// package; the cached copy keeps the same name.
func piTarballName() string {
	return "pi-coding-agent-" + PiVersion + ".tgz"
}

// piTarballURL is the npm registry tarball URL of the pinned pi package.
var piTarballURL = "https://registry.npmjs.org/@earendil-works/pi-coding-agent/-/" + piTarballName()

// piIntegrity is the registry-published dist.integrity (sha512) of the
// pinned pi tarball.
const piIntegrity = "sha512-ncAqFrG+iybuPGOhMiZoEHkEzTpJgz3guYD32pD+M7ucc0WeHmauP6wa7qwP8V/KWvsZDVNa5XGsdZ7fkC7w7A=="

// codexReleaseTag is the GitHub release tag carrying the pinned Codex CLI.
const codexReleaseTag = "rust-v0.147.0"

// openCodeReleaseTag is the GitHub release tag carrying the pinned OpenCode.
const openCodeReleaseTag = "v1.18.18"

// Error is a stable, code-carrying failure.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Msg
}

// archiveKind is the compression of one download target.
type archiveKind int

const (
	archiveTarGz archiveKind = iota
	archiveZip
)

// platform describes one supported download target.
type platform struct {
	archive string
	sha256  string
	binary  string
	kind    archiveKind
}

// codexPlatforms maps GOOS/GOARCH to the pinned Codex package archive. The
// checksums are the values published in the release's codex-package_SHA256SUMS.
var codexPlatforms = map[string]platform{
	"linux/amd64": {
		archive: "codex-package-x86_64-unknown-linux-musl.tar.gz",
		sha256:  "bd758d53d56e41dc65e045f4589df79a038ed197a011adcb52a258e6ad64cfda",
		binary:  "bin/codex",
		kind:    archiveTarGz,
	},
	"linux/arm64": {
		archive: "codex-package-aarch64-unknown-linux-musl.tar.gz",
		sha256:  "89cbf79bd5ae6f9c58da47e8079f311c84219350c9c43c070d42f3e9b2a81401",
		binary:  "bin/codex",
		kind:    archiveTarGz,
	},
	"darwin/amd64": {
		archive: "codex-package-x86_64-apple-darwin.tar.gz",
		sha256:  "d91e59133daf923bc45d76e3da4af8ae9ef62a0231da18488da0cd573b6e9d63",
		binary:  "bin/codex",
		kind:    archiveTarGz,
	},
	"darwin/arm64": {
		archive: "codex-package-aarch64-apple-darwin.tar.gz",
		sha256:  "17b2984eb22b607e3d0c25728252fc90f510e476bad39a6d9f45cdb1aa685432",
		binary:  "bin/codex",
		kind:    archiveTarGz,
	},
	"windows/amd64": {
		archive: "codex-package-x86_64-pc-windows-msvc.tar.gz",
		sha256:  "c156c8feb8cb20197bf74d2c6daffed1fec0a8c21a03bc2ca90d7ff81927b0c5",
		binary:  "bin/codex.exe",
		kind:    archiveTarGz,
	},
	"windows/arm64": {
		archive: "codex-package-aarch64-pc-windows-msvc.tar.gz",
		sha256:  "4533928d72ac4d7c19f16e8c4acdfd02dc255d2aeeb2f6d7dfd45493ec4c0806",
		binary:  "bin/codex.exe",
		kind:    archiveTarGz,
	},
}

// openCodePlatforms maps GOOS/GOARCH to the pinned OpenCode package archive.
// The checksums are the values published in the v1.18.18 release digests.
var openCodePlatforms = map[string]platform{
	"linux/amd64": {
		archive: "opencode-linux-x64.tar.gz",
		sha256:  "0cddc222418b8553669905a8980c0cda7088f00da24d83d6ac76b01c9fdb2aaf",
		binary:  "opencode",
		kind:    archiveTarGz,
	},
	"linux/arm64": {
		archive: "opencode-linux-arm64.tar.gz",
		sha256:  "dcb1b5ec5687b43f87749560021f9203f3809e0ce5ae44ff9be8ae17083fe4ba",
		binary:  "opencode",
		kind:    archiveTarGz,
	},
	"darwin/amd64": {
		archive: "opencode-darwin-x64.zip",
		sha256:  "9581bd7683a7528456179fb11e3377d9ef568e10a935611a2c6722e349454d83",
		binary:  "opencode",
		kind:    archiveZip,
	},
	"darwin/arm64": {
		archive: "opencode-darwin-arm64.zip",
		sha256:  "7d668bf26496fec8686d4e51ebb1ac2bd2e393f0c1620aa696c4c242a9e5806a",
		binary:  "opencode",
		kind:    archiveZip,
	},
	"windows/amd64": {
		archive: "opencode-windows-x64.zip",
		sha256:  "c6d265376fdb93164013671b0cf402410184f73c34fc15d82d40a16a745b15f4",
		binary:  "opencode.exe",
		kind:    archiveZip,
	},
	"windows/arm64": {
		archive: "opencode-windows-arm64.zip",
		sha256:  "0d34d837ea3b5e10349d8550318083040a8b4c061d3faaa4eabd339984aa49b0",
		binary:  "opencode.exe",
		kind:    archiveZip,
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
	return cachedBinaryPath("codex", "Codex", CodexVersion, codexPlatforms)
}

// OpenCodeBinaryPath returns the path of the cached pinned OpenCode
// executable, validating that it exists, is executable, and matches the
// recorded checksum. It never downloads or installs. When the cached
// prerequisite is absent or invalid it returns an *Error with the exact
// setup remediation.
func OpenCodeBinaryPath() (string, error) {
	return cachedBinaryPath("opencode", "OpenCode", OpenCodeVersion, openCodePlatforms)
}

// PiTreePath returns the path of the cached pinned pi CLI entry point,
// validating that the cached npm tree exists and matches the recorded
// integrity. It never downloads or installs. When the cached prerequisite
// is absent or invalid it returns an *Error with the exact setup
// remediation.
func PiTreePath() (string, error) {
	dir, err := CacheDir()
	if err != nil {
		return "", err
	}
	entryDir := filepath.Join(dir, runtimeDir("pi", PiVersion))
	if err := validatePiTree(entryDir); err != nil {
		return "", &Error{Code: "E300", Msg: fmt.Sprintf("cached pi %s is absent or invalid (%v); run `learning-loop runtime-setup pi`", PiVersion, err)}
	}
	return filepath.Join(entryDir, PiEntryPoint), nil
}

// SetupCodex downloads the pinned Codex CLI, verifies its published
// checksum, and stores it in the development Runtime cache. It is
// idempotent: an already valid cached binary is left untouched.
func SetupCodex() error {
	return setupRuntime("openai/codex", "codex", CodexVersion, codexReleaseTag, deps{
		platform: func() (platform, error) {
			return currentPlatform(codexPlatforms, "Codex "+CodexVersion)
		},
		download: downloadArchive,
	})
}

// SetupOpenCode downloads the pinned OpenCode, verifies its published
// checksum, and stores it in the development Runtime cache. It is
// idempotent: an already valid cached binary is left untouched.
func SetupOpenCode() error {
	return setupRuntime("sst/opencode", "opencode", OpenCodeVersion, openCodeReleaseTag, deps{
		platform: func() (platform, error) {
			return currentPlatform(openCodePlatforms, "OpenCode "+OpenCodeVersion)
		},
		download: downloadArchive,
	})
}

// SetupPi downloads the pinned pi npm package, verifies its registry
// integrity, extracts it into the development Runtime cache, and installs
// its shrinkwrap-pinned dependencies so the cached tree is runnable. It is
// idempotent: an already valid cached tree is left untouched.
func SetupPi() error {
	return setupPi(piDeps{
		download:  downloadArchive,
		integrity: func(archive []byte) error { return verifyIntegrity(archive, piIntegrity) },
		install:   npmInstall,
	})
}

type piDeps struct {
	download  func(url string) ([]byte, error)
	integrity func(archive []byte) error
	install   func(dir string) error
}

func setupPi(d piDeps) error {
	cacheDir, err := CacheDir()
	if err != nil {
		return err
	}
	target := filepath.Join(cacheDir, runtimeDir("pi", PiVersion))
	if err := validatePiTree(target); err == nil {
		return nil
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("creating the Runtime cache: %v", err)}
	}

	// Stage the complete cache entry outside the live path. A failed download,
	// integrity check, extraction, or install therefore cannot damage an
	// existing entry.
	staging, err := os.MkdirTemp(cacheDir, ".pi-staging-")
	if err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("creating the Runtime cache staging area: %v", err)}
	}
	defer os.RemoveAll(staging)

	archive, err := d.download(piTarballURL)
	if err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("downloading %s: %v", piTarballURL, err)}
	}
	if err := d.integrity(archive); err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("integrity mismatch for %s; refusing to cache it", piTarballName())}
	}
	if err := extractTarGzTree(archive, staging); err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("extracting %s: %v", piTarballName(), err)}
	}
	// The published pi package is not self-contained: its runtime
	// dependencies are pinned by the shrinkwrap inside the verified tarball
	// and installed here once, so conformance never downloads.
	if err := d.install(filepath.Join(staging, "package")); err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("installing the pinned pi dependencies: %v", err)}
	}
	if err := writeIntegrityRecord(staging, piTarballName(), archive); err != nil {
		return err
	}
	if err := replaceCacheEntry(target, staging); err != nil {
		return err
	}
	return nil
}

// npmInstall installs the production dependencies of the package rooted at
// dir exactly as pinned by its npm-shrinkwrap.json, without rewriting the
// lockfile.
func npmInstall(dir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "npm", "install", "--omit=dev", "--no-audit", "--no-fund", "--no-package-lock")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 1024 {
			msg = msg[:1024] + "... (truncated)"
		}
		return fmt.Errorf("npm install: %v: %s", err, msg)
	}
	return nil
}

// verifyIntegrity checks archive against an npm SRI integrity string of the
// form "sha512-<base64 digest>".
func verifyIntegrity(archive []byte, integrity string) error {
	const prefix = "sha512-"
	if !strings.HasPrefix(integrity, prefix) {
		return fmt.Errorf("unsupported integrity format %q", integrity)
	}
	sum := sha512.Sum512(archive)
	if base64.StdEncoding.EncodeToString(sum[:]) != strings.TrimPrefix(integrity, prefix) {
		return fmt.Errorf("integrity mismatch")
	}
	return nil
}

// extractTarGzTree extracts a gzipped tar archive into dir, rejecting any
// entry that would escape dir.
func extractTarGzTree(archive []byte, dir string) error {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(hdr.Name)
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) || strings.HasPrefix(name, "../") {
			return fmt.Errorf("unsafe archive entry %q", hdr.Name)
		}
		target := filepath.Join(dir, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		default:
			// Skip device, fifo, and other special entries; the pinned
			// package contains only regular files, directories, and links.
		}
	}
}

// writeIntegrityRecord stores the verified tarball and its recorded
// integrity next to the extracted tree, so later validation can confirm the
// cached tree derives from the pinned artifact.
func writeIntegrityRecord(dir, name string, archive []byte) error {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, archive, 0o644); err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("recording the cached tarball: %v", err)}
	}
	sum := sha512.Sum512(archive)
	record := base64.StdEncoding.EncodeToString(sum[:]) + "\n"
	if err := os.WriteFile(path+".sha512", []byte(record), 0o644); err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("recording the cached integrity: %v", err)}
	}
	return nil
}

// validatePiTree checks that the cached pi entry holds the extracted npm
// tree with its installed dependencies, the verified tarball, and a
// recorded integrity matching the tarball.
func validatePiTree(entryDir string) error {
	entryPoint := filepath.Join(entryDir, PiEntryPoint)
	info, err := os.Stat(entryPoint)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("entry point is not a regular file")
	}
	recorded, err := os.ReadFile(filepath.Join(entryDir, piTarballName()+".sha512"))
	if err != nil {
		return fmt.Errorf("missing recorded integrity")
	}
	content, err := os.ReadFile(filepath.Join(entryDir, piTarballName()))
	if err != nil {
		return err
	}
	sum := sha512.Sum512(content)
	if base64.StdEncoding.EncodeToString(sum[:]) != strings.TrimSpace(string(recorded)) {
		return fmt.Errorf("integrity mismatch")
	}
	if err := validateInstalledDeps(entryDir); err != nil {
		return err
	}
	return nil
}

// validateInstalledDeps checks that every required dependency of the pinned
// package is present in the cached tree's node_modules, so the tree is
// runnable without further downloads. Optional dependencies are excluded
// because npm may skip them on some platforms.
func validateInstalledDeps(entryDir string) error {
	data, err := os.ReadFile(filepath.Join(entryDir, "package", "package.json"))
	if err != nil {
		return fmt.Errorf("missing package.json: %v", err)
	}
	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return fmt.Errorf("parsing package.json: %v", err)
	}
	for name := range pkg.Dependencies {
		if _, err := os.Stat(filepath.Join(entryDir, "package", "node_modules", name)); err != nil {
			return fmt.Errorf("missing installed dependency %s", name)
		}
	}
	return nil
}

type deps struct {
	platform func() (platform, error)
	download func(url string) ([]byte, error)
}

// runtimeDir returns the cache subdirectory for one pinned Runtime.
func runtimeDir(name, version string) string {
	return name + "-" + version
}

func cachedBinaryPath(name, display, version string, platforms map[string]platform) (string, error) {
	dir, err := CacheDir()
	if err != nil {
		return "", err
	}
	p, err := currentPlatform(platforms, display+" "+version)
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, runtimeDir(name, version), filepath.Base(p.binary))
	if err := validateBinary(bin); err != nil {
		return "", &Error{Code: "E300", Msg: fmt.Sprintf("cached %s %s is absent or invalid (%v); run `learning-loop runtime-setup %s`", display, version, err, name)}
	}
	return bin, nil
}

func setupRuntime(repo, name, version, tag string, d deps) error {
	p, err := d.platform()
	if err != nil {
		return err
	}
	cacheDir, err := CacheDir()
	if err != nil {
		return err
	}
	target := filepath.Join(cacheDir, runtimeDir(name, version))
	bin := filepath.Join(target, filepath.Base(p.binary))
	if err := validateBinary(bin); err == nil {
		return nil
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("creating the Runtime cache: %v", err)}
	}

	// Stage the complete cache entry outside the live path. A failed download,
	// checksum, extraction, or write therefore cannot damage an existing entry.
	staging, err := os.MkdirTemp(cacheDir, "."+name+"-staging-")
	if err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("creating the Runtime cache staging area: %v", err)}
	}
	defer os.RemoveAll(staging)

	url := "https://github.com/" + repo + "/releases/download/" + tag + "/" + p.archive
	archive, err := d.download(url)
	if err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("downloading %s: %v", url, err)}
	}
	if sum := sha256.Sum256(archive); hex.EncodeToString(sum[:]) != p.sha256 {
		return &Error{Code: "E301", Msg: fmt.Sprintf("checksum mismatch for %s; refusing to cache it", p.archive)}
	}
	content, err := extractEntry(archive, p.binary, p.kind)
	if err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("extracting %s from %s: %v", p.binary, p.archive, err)}
	}
	stagedBin := filepath.Join(staging, filepath.Base(p.binary))
	if err := writeBinary(staging, stagedBin, content); err != nil {
		return err
	}
	if err := replaceCacheEntry(target, staging); err != nil {
		return err
	}
	return nil
}

// replaceCacheEntry swaps a fully written staged directory into the live
// cache path. The old entry is moved aside first so a replacement failure can
// be rolled back without exposing a partially written cache entry.
func replaceCacheEntry(target, staging string) error {
	parent := filepath.Dir(target)
	backup, err := os.MkdirTemp(parent, ".cache-backup-")
	if err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("preparing the Runtime cache replacement: %v", err)}
	}
	if err := os.Remove(backup); err != nil {
		return &Error{Code: "E301", Msg: fmt.Sprintf("preparing the Runtime cache replacement: %v", err)}
	}

	oldMoved := false
	if err := os.Rename(target, backup); err != nil {
		if !os.IsNotExist(err) {
			return &Error{Code: "E301", Msg: fmt.Sprintf("replacing the Runtime cache: %v", err)}
		}
	} else {
		oldMoved = true
	}
	if err := os.Rename(staging, target); err != nil {
		if oldMoved {
			_ = os.Rename(backup, target)
		}
		return &Error{Code: "E301", Msg: fmt.Sprintf("replacing the Runtime cache: %v", err)}
	}
	if oldMoved {
		_ = os.RemoveAll(backup)
	}
	return nil
}

// currentPlatform resolves the download target for the running platform.
func currentPlatform(platforms map[string]platform, label string) (platform, error) {
	p, ok := platforms[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok {
		return platform{}, &Error{Code: "E306", Msg: fmt.Sprintf("no pinned %s archive for %s/%s", label, runtime.GOOS, runtime.GOARCH)}
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

// extractEntry returns the bytes of the named entry inside a tar.gz or zip
// archive.
func extractEntry(archive []byte, name string, kind archiveKind) ([]byte, error) {
	if kind == archiveZip {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if f.Name == name {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, fmt.Errorf("entry %q not found", name)
	}
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
	tmp, err := os.CreateTemp(target, ".runtime-*")
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
	sumTmp, err := os.CreateTemp(target, ".runtime-*")
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
