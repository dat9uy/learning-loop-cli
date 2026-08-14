package runtimecache

import (
	"archive/tar"
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

func fakeArchive(t *testing.T, binary []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "bin/codex", Mode: 0o755, Size: int64(len(binary))}); err != nil {
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

func fakePlatform(archive []byte) platform {
	sum := sha256.Sum256(archive)
	return platform{archive: "codex-package-x86_64-unknown-linux-musl.tar.gz", sha256: hex.EncodeToString(sum[:]), binary: "bin/codex"}
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
	archive := fakeArchive(t, []byte("#!/bin/sh\necho fake codex\n"))
	p := fakePlatform(archive)
	dir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", dir)

	err := setupCodex(deps{
		platform: func() (platform, error) { return p, nil },
		download: func(url string) ([]byte, error) {
			if !strings.Contains(url, "rust-v0.147.0") || !strings.Contains(url, p.archive) {
				t.Fatalf("download url = %q", url)
			}
			return archive, nil
		},
	})
	if err != nil {
		t.Fatalf("setupCodex: %v", err)
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
	archive := fakeArchive(t, []byte("#!/bin/sh\necho fake codex\n"))
	p := fakePlatform(archive)
	dir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", dir)
	d := deps{
		platform: func() (platform, error) { return p, nil },
		download: func(url string) ([]byte, error) { return archive, nil },
	}
	if err := setupCodex(d); err != nil {
		t.Fatalf("first setupCodex: %v", err)
	}
	downloads := 0
	d.download = func(url string) ([]byte, error) {
		downloads++
		return archive, nil
	}
	if err := setupCodex(d); err != nil {
		t.Fatalf("second setupCodex: %v", err)
	}
	if downloads != 0 {
		t.Fatalf("second setup downloaded %d times, want 0", downloads)
	}
}

func TestSetupCodexRejectsChecksumMismatch(t *testing.T) {
	archive := fakeArchive(t, []byte("#!/bin/sh\necho fake codex\n"))
	p := fakePlatform(archive)
	p.sha256 = strings.Repeat("0", 64)
	dir := t.TempDir()
	t.Setenv("LEARNING_LOOP_CACHE", dir)

	err := setupCodex(deps{
		platform: func() (platform, error) { return p, nil },
		download: func(url string) ([]byte, error) { return archive, nil },
	})
	if err == nil {
		t.Fatalf("setupCodex succeeded with a bad checksum")
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
	err := setupCodex(deps{
		platform: func() (platform, error) {
			return platform{}, &Error{Code: "E306", Msg: "no pinned Codex 0.147.0 archive for plan9/amd64"}
		},
		download: func(url string) ([]byte, error) { return nil, nil },
	})
	if err == nil || !strings.Contains(err.Error(), "E306") {
		t.Fatalf("error = %v, want E306", err)
	}
}
