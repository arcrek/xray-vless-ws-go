package selfupdate

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRollbackSwapsBinaries(t *testing.T) {
	execPath := withFakeExecutable(t, "current (v2)")
	if err := os.WriteFile(execPath+".prev", []byte("previous (v1)"), 0755); err != nil {
		t.Fatal(err)
	}
	withFakeRestart(t, true, true)

	var out bytes.Buffer
	if err := Rollback(contextBG(), &out); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	got, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "previous (v1)" {
		t.Errorf("execPath = %q, want %q", got, "previous (v1)")
	}
	if _, err := os.Stat(execPath + ".prev"); !os.IsNotExist(err) {
		t.Errorf(".prev should be consumed by rollback, stat err = %v", err)
	}
}

func TestRollbackNoPrevBinary(t *testing.T) {
	withFakeExecutable(t, "current")
	withFakeRestart(t, true, true)

	var out bytes.Buffer
	err := Rollback(contextBG(), &out)
	if err == nil {
		t.Fatal("expected error when no .prev exists, got nil")
	}
	if !strings.Contains(err.Error(), "no previous binary to roll back to") {
		t.Errorf("error = %q, want the specific no-prev message", err.Error())
	}
}

func TestRollbackTwiceFailsCleanlyOnSecondAttempt(t *testing.T) {
	execPath := withFakeExecutable(t, "current (v2)")
	if err := os.WriteFile(execPath+".prev", []byte("previous (v1)"), 0755); err != nil {
		t.Fatal(err)
	}
	withFakeRestart(t, true, true)

	var out bytes.Buffer
	if err := Rollback(contextBG(), &out); err != nil {
		t.Fatalf("first Rollback: %v", err)
	}
	if err := Rollback(contextBG(), &out); err == nil {
		t.Fatal("expected the second consecutive Rollback to fail (single-level only)")
	}
}

func TestRollbackNoSystemdUnitIsCleanSuccess(t *testing.T) {
	execPath := withFakeExecutable(t, "current (v2)")
	os.WriteFile(execPath+".prev", []byte("previous (v1)"), 0755)
	withFakeRestart(t, false, true)

	var out bytes.Buffer
	if err := Rollback(contextBG(), &out); err != nil {
		t.Fatalf("Rollback should succeed cleanly with no systemd unit, got: %v", err)
	}
	if !strings.Contains(out.String(), "no systemd unit detected") {
		t.Errorf("output = %q, want a manual-restart message", out.String())
	}
}

func TestRollbackRestartFailsAfterSuccessfulSwap(t *testing.T) {
	execPath := withFakeExecutable(t, "current (v2)")
	os.WriteFile(execPath+".prev", []byte("previous (v1)"), 0755)
	withFakeRestart(t, true, false)

	var out bytes.Buffer
	err := Rollback(contextBG(), &out)
	if err == nil {
		t.Fatal("expected a non-nil error when restart fails after a successful rollback swap")
	}
	if !strings.Contains(err.Error(), "rolled back") || !strings.Contains(err.Error(), "restart failed") {
		t.Errorf("error = %q, want partial-success framing", err.Error())
	}

	got, readErr := os.ReadFile(execPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "previous (v1)" {
		t.Errorf("execPath = %q, want the swap to have completed despite the restart failure", got)
	}
}
