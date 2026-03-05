package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestIsCaddyATCProcess_CurrentProcess(t *testing.T) {
	// Our own process won't be named "caddy-atc", but the function
	// should not panic or error on a valid PID
	pid := os.Getpid()
	// Should return false since test binary isn't named caddy-atc
	if isCaddyATCProcess(pid) {
		t.Error("isCaddyATCProcess(self) = true, want false for test binary")
	}
}

func TestIsCaddyATCProcess_NonExistentPID(t *testing.T) {
	// PID that almost certainly doesn't exist
	got := isCaddyATCProcess(9999999)
	if got {
		t.Error("isCaddyATCProcess(9999999) = true, want false for non-existent PID")
	}
}

func TestWaitForPidFile_Success(t *testing.T) {
	// Create a temp dir and write a PID file after a short delay
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "test.pid")

	// Override config.PidPath for this test by writing the file directly
	// We test waitForPidFile by writing the PID file in a goroutine
	go func() {
		time.Sleep(200 * time.Millisecond)
		os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0644)
	}()

	err := waitForPidFileAt(pidFile, 2*time.Second)
	if err != nil {
		t.Errorf("waitForPidFileAt() returned error: %v", err)
	}
}

func TestWaitForPidFile_Timeout(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "nonexistent.pid")

	err := waitForPidFileAt(pidFile, 300*time.Millisecond)
	if err == nil {
		t.Error("waitForPidFileAt() returned nil, want timeout error")
	}
}

func TestWaitForPidFile_InvalidContent(t *testing.T) {
	// PID file exists but has non-numeric content; should wait until timeout
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "bad.pid")
	os.WriteFile(pidFile, []byte("not-a-pid"), 0644)

	// Write valid content after a delay
	go func() {
		time.Sleep(200 * time.Millisecond)
		os.WriteFile(pidFile, []byte("12345"), 0644)
	}()

	err := waitForPidFileAt(pidFile, 2*time.Second)
	if err != nil {
		t.Errorf("waitForPidFileAt() returned error: %v", err)
	}
}

func TestUpCmd_Flags(t *testing.T) {
	cmd := upCmd()

	// --detach / -d should exist and be visible
	f := cmd.Flags().Lookup("detach")
	if f == nil {
		t.Fatal("--detach flag not found")
	}
	if f.Shorthand != "d" {
		t.Errorf("--detach shorthand = %q, want %q", f.Shorthand, "d")
	}
	if f.DefValue != "false" {
		t.Errorf("--detach default = %q, want %q", f.DefValue, "false")
	}

	// --_daemon should exist but be hidden
	d := cmd.Flags().Lookup("_daemon")
	if d == nil {
		t.Fatal("--_daemon flag not found")
	}
	if d.Hidden != true {
		t.Error("--_daemon should be hidden")
	}
}
