package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestFindAvailablePort(t *testing.T) {
	port, err := findAvailablePort()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if port <= 0 || port > 65535 {
		t.Errorf("expected valid port, got: %d", port)
	}
}

func TestResolvePort_AutoSelect(t *testing.T) {
	port, err := resolvePort(0)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if port <= 0 || port > 65535 {
		t.Errorf("expected valid port, got: %d", port)
	}
}

func TestResolvePort_Requested(t *testing.T) {
	// Get a free port, then request it — it should be returned as-is.
	free, err := findAvailablePort()
	if err != nil {
		t.Fatalf("could not find free port: %v", err)
	}
	got, err := resolvePort(free)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got != free {
		t.Errorf("expected port %d, got %d", free, got)
	}
}

func TestResolvePort_BusyFallsBack(t *testing.T) {
	// Hold a port open, then ask resolvePort to use it — it should fall back to a different port.
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("could not bind port: %v", err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port

	got, err := resolvePort(busy)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got == busy {
		t.Errorf("expected a different port than busy port %d, but got the same", busy)
	}
	if got <= 0 || got > 65535 {
		t.Errorf("expected valid fallback port, got: %d", got)
	}
}

func TestDetectProviderDirs_AllPresent(t *testing.T) {
	root := t.TempDir()
	for _, sub := range providerSubdirs {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
	}
	dirs := detectProviderDirs(root)
	if len(dirs) != len(providerSubdirs) {
		t.Errorf("expected %d dirs, got %d", len(providerSubdirs), len(dirs))
	}
}

func TestDetectProviderDirs_SomeMissing(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	dirs := detectProviderDirs(root)
	if len(dirs) != 1 {
		t.Errorf("expected 1 dir, got %d", len(dirs))
	}
	if filepath.Base(dirs[0]) != ".claude" {
		t.Errorf("expected .claude, got %s", dirs[0])
	}
}

func TestDetectProviderDirs_NonePresent(t *testing.T) {
	root := t.TempDir()
	dirs := detectProviderDirs(root)
	if len(dirs) != 0 {
		t.Errorf("expected 0 dirs, got %d", len(dirs))
	}
}

func TestDetectProviderDirs_OverriddenRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".gemini"), 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	dirs := detectProviderDirs(root)
	if len(dirs) != 1 {
		t.Errorf("expected 1 dir, got %d", len(dirs))
	}
}

func TestDefaultLogsDir(t *testing.T) {
	dir := defaultLogsDir()
	if dir == "" {
		t.Error("expected non-empty logs dir")
	}
}
