package main

import (
	"os"
	"testing"
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
