package selfupdate

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"testing"
)

func TestBuildRestartCommand(t *testing.T) {
	if got := buildRestartCommand(true); !reflect.DeepEqual(got, []string{"systemctl", "restart", "xrayws"}) {
		t.Errorf("root: got %v", got)
	}
	if got := buildRestartCommand(false); !reflect.DeepEqual(got, []string{"sudo", "systemctl", "restart", "xrayws"}) {
		t.Errorf("non-root: got %v", got)
	}
}

func TestRestartServiceNoUnitDetected(t *testing.T) {
	oldIsUnitPresent := isUnitPresent
	isUnitPresent = func(ctx context.Context) bool { return false }
	defer func() { isUnitPresent = oldIsUnitPresent }()

	err := restartService(context.Background())
	if !errors.Is(err, errNoSystemdUnit) {
		t.Fatalf("expected errNoSystemdUnit, got %v", err)
	}
}

func TestRestartServiceInvokesCommand(t *testing.T) {
	oldIsUnitPresent, oldExecCommand := isUnitPresent, execCommand
	defer func() { isUnitPresent, execCommand = oldIsUnitPresent, oldExecCommand }()

	isUnitPresent = func(ctx context.Context) bool { return true }

	var invoked []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		invoked = append([]string{name}, args...)
		// "true" always succeeds; substitutes for the real systemctl call.
		return exec.Command("true")
	}

	if err := restartService(context.Background()); err != nil {
		t.Fatalf("restartService: %v", err)
	}
	if len(invoked) == 0 {
		t.Fatal("execCommand was never invoked")
	}
}
