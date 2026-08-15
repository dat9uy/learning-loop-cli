package runtimecache

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeTarGz(t *testing.T, name string, binary []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(binary))}); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	if _, err := tw.Write(binary); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("Close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("Close gzip: %v", err)
	}
	return buf.Bytes()
}

func fakeZip(t *testing.T, name string, binary []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := w.Write(binary); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close zip: %v", err)
	}
	return buf.Bytes()
}

func fakePlatform(archive []byte, p platform) platform {
	sum := sha256.Sum256(archive)
	p.sha256 = hex.EncodeToString(sum[:])
	return p
}

func TestCacheDirResolution(t *testing.T) {
	t.Setenv("LEARNING_LOOP_CACHE", "/tmp/ll-cache")
	if got, _ := CacheDir(); got != "/tmp/ll-cache" {
		t.Fatalf("CacheDir with LEARNING_LOOP_CACHE = %q", got)
	}
	t.Setenv("LEARNING_LOOP_CACHE", "")
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg")
	if got, _ := CacheDir(); got != filepath.Join("/tmp/xdg", "learning-loop", "runtimes") {
		t.Fatalf("CacheDir with XDG_CACHE_HOME = %q", got)
	}
}

func TestSetupCodexDownloadsVerifiesAndCaches(t *testing.T) {
	archive := fakeTarGz(t, "bin/codex", []byte("#!/bin/sh\necho fake codex\n"))
	p := fakePlatform(archive, platform{archive: "codex-package-x86_64-unknown-linux-musl.tar.gz", binary: "bin/codex", kind: archiveTarGz})
	dir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", dir)

	err := setupRuntime("openai/codex", "codex", CodexVersion, codexReleaseTag, deps{
		platform: func() (platform, error) { return p, nil },
		download: func(url string) ([]byte, error) {
			if !strings.Contains(url, "rust-v0.147.0") || !strings.Contains(url, p.archive) {
				t.Fatalf("download url = %q", url)
			}
			return archive, nil
		},
	})
	if err != nil {
		t.Fatalf("setupRuntime: %v", err)
	}
	bin := filepath.Join(dir, "codex-"+CodexVersion, "codex")
	data, err := os.ReadFile(bin)
	if err != nil {
		t.Fatalf("cached binary: %v", err)
	}
	if !strings.Contains(string(data), "fake codex") {
		t.Fatalf("cached binary = %q", data)
	}
	if _, err := CodexBinaryPath(); err != nil {
		t.Fatalf("CodexBinaryPath after setup: %v", err)
	}
}

func TestSetupCodexIsIdempotent(t *testing.T) {
	archive := fakeTarGz(t, "bin/codex", []byte("#!/bin/sh\necho fake codex\n"))
	p := fakePlatform(archive, platform{archive: "codex-package-x86_64-unknown-linux-musl.tar.gz", binary: "bin/codex", kind: archiveTarGz})
	dir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", dir)
	d := deps{
		platform: func() (platform, error) { return p, nil },
		download: func(url string) ([]byte, error) { return archive, nil },
	}
	if err := setupRuntime("openai/codex", "codex", CodexVersion, codexReleaseTag, d); err != nil {
		t.Fatalf("first setupRuntime: %v", err)
	}
	downloads := 0
	d.download = func(url string) ([]byte, error) {
		downloads++
		return archive, nil
	}
	if err := setupRuntime("openai/codex", "codex", CodexVersion, codexReleaseTag, d); err != nil {
		t.Fatalf("second setupRuntime: %v", err)
	}
	if downloads != 0 {
		t.Fatalf("second setup downloaded %d times, want 0", downloads)
	}
}

func TestSetupCodexPreservesExistingEntryOnChecksumFailure(t *testing.T) {
	archive := fakeTarGz(t, "bin/codex", []byte("#!/bin/sh\necho replacement\n"))
	p := fakePlatform(archive, platform{archive: "codex-package-x86_64-unknown-linux-musl.tar.gz", binary: "bin/codex", kind: archiveTarGz})
	p.sha256 = strings.Repeat("0", 64)
	dir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", dir)
	old := []byte("#!/bin/sh\necho existing\n")
	populate := filepath.Join(dir, "codex-"+CodexVersion)
	if err := os.MkdirAll(populate, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	bin := filepath.Join(populate, "codex")
	if err := os.WriteFile(bin, old, 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(bin+".sha256", []byte(strings.Repeat("0", 64)+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile checksum: %v", err)
	}

	err := setupRuntime("openai/codex", "codex", CodexVersion, codexReleaseTag, deps{
		platform: func() (platform, error) { return p, nil },
		download: func(url string) ([]byte, error) { return archive, nil },
	})
	if err == nil {
		t.Fatalf("setupRuntime succeeded with a bad checksum")
	}
	got, readErr := os.ReadFile(bin)
	if readErr != nil || string(got) != string(old) {
		t.Fatalf("existing cache = %q, %v; want the untouched old entry", got, readErr)
	}
}

func TestSetupCodexRejectsChecksumMismatch(t *testing.T) {
	archive := fakeTarGz(t, "bin/codex", []byte("#!/bin/sh\necho fake codex\n"))
	p := fakePlatform(archive, platform{archive: "codex-package-x86_64-unknown-linux-musl.tar.gz", binary: "bin/codex", kind: archiveTarGz})
	p.sha256 = strings.Repeat("0", 64)
	dir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", dir)

	err := setupRuntime("openai/codex", "codex", CodexVersion, codexReleaseTag, deps{
		platform: func() (platform, error) { return p, nil },
		download: func(url string) ([]byte, error) { return archive, nil },
	})
	if err == nil {
		t.Fatalf("setupRuntime succeeded with a bad checksum")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "codex-"+CodexVersion, "codex")); !os.IsNotExist(statErr) {
		t.Fatalf("binary cached despite checksum mismatch")
	}
}

func TestCodexBinaryPathReportsRemediationWhenAbsent(t *testing.T) {
	t.Setenv("LEARNING_LOOP_CACHE", t.TempDir())
	_, err := CodexBinaryPath()
	if err == nil {
		t.Fatalf("CodexBinaryPath succeeded with an empty cache")
	}
	var rerr *Error
	if !errors.As(err, &rerr) || rerr.Code != "E300" {
		t.Fatalf("error = %v, want E300", err)
	}
	if !strings.Contains(err.Error(), "learning-loop runtime-setup codex") {
		t.Fatalf("error = %v, want the exact setup remediation", err)
	}
}

func TestCodexBinaryPathRejectsInvalidCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", dir)
	target := filepath.Join(dir, "codex-"+CodexVersion)
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	bin := filepath.Join(target, "codex")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(bin+".sha256", []byte("deadbeef\n"), 0o644); err != nil {
		t.Fatalf("WriteFile checksum: %v", err)
	}
	_, err := CodexBinaryPath()
	if err == nil || !strings.Contains(err.Error(), "E300") {
		t.Fatalf("error = %v, want E300 for a checksum-mismatched cache", err)
	}
}

func TestSetupCodexUnsupportedPlatform(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", dir)
	err := setupRuntime("openai/codex", "codex", CodexVersion, codexReleaseTag, deps{
		platform: func() (platform, error) {
			return platform{}, &Error{Code: "E306", Msg: "no pinned Codex 0.147.0 archive for plan9/amd64"}
		},
		download: func(url string) ([]byte, error) { return nil, nil },
	})
	if err == nil || !strings.Contains(err.Error(), "E306") {
		t.Fatalf("error = %v, want E306", err)
	}
}

func TestSetupOpenCodeDownloadsVerifiesAndCaches(t *testing.T) {
	archive := fakeZip(t, "opencode", []byte("#!/bin/sh\necho fake opencode\n"))
	p := fakePlatform(archive, platform{archive: "opencode-darwin-arm64.zip", binary: "opencode", kind: archiveZip})
	dir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", dir)

	err := setupRuntime("sst/opencode", "opencode", OpenCodeVersion, openCodeReleaseTag, deps{
		platform: func() (platform, error) { return p, nil },
		download: func(url string) ([]byte, error) {
			if !strings.Contains(url, "v1.18.18") || !strings.Contains(url, p.archive) {
				t.Fatalf("download url = %q", url)
			}
			return archive, nil
		},
	})
	if err != nil {
		t.Fatalf("setupRuntime: %v", err)
	}
	bin := filepath.Join(dir, "opencode-"+OpenCodeVersion, "opencode")
	data, err := os.ReadFile(bin)
	if err != nil {
		t.Fatalf("cached binary: %v", err)
	}
	if !strings.Contains(string(data), "fake opencode") {
		t.Fatalf("cached binary = %q", data)
	}
	if _, err := OpenCodeBinaryPath(); err != nil {
		t.Fatalf("OpenCodeBinaryPath after setup: %v", err)
	}
}

func TestSetupOpenCodeIsIdempotent(t *testing.T) {
	archive := fakeZip(t, "opencode", []byte("#!/bin/sh\necho fake opencode\n"))
	p := fakePlatform(archive, platform{archive: "opencode-darwin-arm64.zip", binary: "opencode", kind: archiveZip})
	dir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", dir)
	d := deps{
		platform: func() (platform, error) { return p, nil },
		download: func(url string) ([]byte, error) { return archive, nil },
	}
	if err := setupRuntime("sst/opencode", "opencode", OpenCodeVersion, openCodeReleaseTag, d); err != nil {
		t.Fatalf("first setupRuntime: %v", err)
	}
	downloads := 0
	d.download = func(url string) ([]byte, error) {
		downloads++
		return archive, nil
	}
	if err := setupRuntime("sst/opencode", "opencode", OpenCodeVersion, openCodeReleaseTag, d); err != nil {
		t.Fatalf("second setupRuntime: %v", err)
	}
	if downloads != 0 {
		t.Fatalf("second setup downloaded %d times, want 0", downloads)
	}
}

func TestSetupOpenCodeRejectsChecksumMismatch(t *testing.T) {
	archive := fakeZip(t, "opencode", []byte("#!/bin/sh\necho fake opencode\n"))
	p := fakePlatform(archive, platform{archive: "opencode-darwin-arm64.zip", binary: "opencode", kind: archiveZip})
	p.sha256 = strings.Repeat("0", 64)
	dir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", dir)

	err := setupRuntime("sst/opencode", "opencode", OpenCodeVersion, openCodeReleaseTag, deps{
		platform: func() (platform, error) { return p, nil },
		download: func(url string) ([]byte, error) { return archive, nil },
	})
	if err == nil {
		t.Fatalf("setupRuntime succeeded with a bad checksum")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "opencode-"+OpenCodeVersion, "opencode")); !os.IsNotExist(statErr) {
		t.Fatalf("binary cached despite checksum mismatch")
	}
}

func TestOpenCodeBinaryPathReportsRemediationWhenAbsent(t *testing.T) {
	t.Setenv("LEARNING_LOOP_CACHE", t.TempDir())
	_, err := OpenCodeBinaryPath()
	if err == nil {
		t.Fatalf("OpenCodeBinaryPath succeeded with an empty cache")
	}
	var rerr *Error
	if !errors.As(err, &rerr) || rerr.Code != "E300" {
		t.Fatalf("error = %v, want E300", err)
	}
	if !strings.Contains(err.Error(), "learning-loop runtime-setup opencode") {
		t.Fatalf("error = %v, want the exact setup remediation", err)
	}
}

func TestOpenCodeBinaryPathRejectsInvalidCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", dir)
	target := filepath.Join(dir, "opencode-"+OpenCodeVersion)
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	bin := filepath.Join(target, "opencode")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(bin+".sha256", []byte("deadbeef\n"), 0o644); err != nil {
		t.Fatalf("WriteFile checksum: %v", err)
	}
	_, err := OpenCodeBinaryPath()
	if err == nil || !strings.Contains(err.Error(), "E300") {
		t.Fatalf("error = %v, want E300 for a checksum-mismatched cache", err)
	}
}

func TestSetupOpenCodeUnsupportedPlatform(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", dir)
	err := setupRuntime("sst/opencode", "opencode", OpenCodeVersion, openCodeReleaseTag, deps{
		platform: func() (platform, error) {
			return platform{}, &Error{Code: "E306", Msg: "no pinned OpenCode 1.18.18 archive for plan9/amd64"}
		},
		download: func(url string) ([]byte, error) { return nil, nil },
	})
	if err == nil || !strings.Contains(err.Error(), "E306") {
		t.Fatalf("error = %v, want E306", err)
	}
}

func TestExtractEntryZip(t *testing.T) {
	archive := fakeZip(t, "opencode.exe", []byte("exe-bytes"))
	got, err := extractEntry(archive, "opencode.exe", archiveZip)
	if err != nil {
		t.Fatalf("extractEntry: %v", err)
	}
	if string(got) != "exe-bytes" {
		t.Fatalf("extracted = %q, want exe-bytes", got)
	}
	if _, err := extractEntry(archive, "missing", archiveZip); err == nil {
		t.Fatalf("extractEntry accepted a missing zip entry")
	}
}
