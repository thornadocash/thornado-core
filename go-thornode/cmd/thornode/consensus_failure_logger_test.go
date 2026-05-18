package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"cosmossdk.io/log"
)

// recordingLogger captures log calls for testing.
type recordingLogger struct {
	log.Logger
	errorCalls []string
}

func newRecordingLogger() *recordingLogger {
	return &recordingLogger{Logger: log.NewNopLogger()}
}

func (r *recordingLogger) Error(msg string, keyvals ...interface{}) {
	r.errorCalls = append(r.errorCalls, msg)
}

func (r *recordingLogger) With(keyvals ...interface{}) log.Logger {
	return r
}

func (r *recordingLogger) Impl() interface{} {
	return nil
}

func TestConsensusFailureLogger_NormalErrors(t *testing.T) {
	inner := newRecordingLogger()
	logger := NewConsensusFailureLogger(inner)

	// Normal error messages should pass through without exiting.
	logger.Error("some error", "key", "value")
	logger.Error("another error")

	if len(inner.errorCalls) != 2 {
		t.Fatalf("expected 2 error calls, got %d", len(inner.errorCalls))
	}
	if inner.errorCalls[0] != "some error" {
		t.Errorf("expected first call to be 'some error', got %q", inner.errorCalls[0])
	}
	if inner.errorCalls[1] != "another error" {
		t.Errorf("expected second call to be 'another error', got %q", inner.errorCalls[1])
	}
}

func TestConsensusFailureLogger_WithPreservesWrapper(t *testing.T) {
	inner := newRecordingLogger()
	logger := NewConsensusFailureLogger(inner)

	// With() should return a ConsensusFailureLogger wrapping the sub-logger.
	subLogger := logger.With("module", "consensus")
	if _, ok := subLogger.(ConsensusFailureLogger); !ok {
		t.Fatalf("expected ConsensusFailureLogger from With(), got %T", subLogger)
	}
}

func TestConsensusFailureLogger_ImplDelegates(t *testing.T) {
	inner := newRecordingLogger()
	logger := NewConsensusFailureLogger(inner)

	// Impl() should delegate to the inner logger.
	if logger.Impl() != nil {
		t.Errorf("expected nil from Impl(), got %v", logger.Impl())
	}
}

// TestConsensusFailureMsg_MatchesCometBFT verifies that our consensusFailureMsg
// constant matches the string literal used in CometBFT's consensus/state.go.
// This will fail if a CometBFT upgrade changes the message, alerting us that
// the wrapper logger would silently stop detecting consensus failures.
func TestConsensusFailureMsg_MatchesCometBFT(t *testing.T) {
	// Locate the CometBFT module directory via `go list`.
	cmd := exec.Command("go", "list", "-m", "-json", "github.com/cometbft/cometbft")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -m -json github.com/cometbft/cometbft: %v", err)
	}

	var mod struct{ Dir string }
	if err = json.Unmarshal(out, &mod); err != nil {
		t.Fatalf("unmarshal go list output: %v", err)
	}
	if mod.Dir == "" {
		t.Fatal("CometBFT module directory is empty — is the module downloaded?")
	}

	src, err := os.ReadFile(filepath.Join(mod.Dir, "consensus", "state.go"))
	if err != nil {
		t.Fatalf("read CometBFT consensus/state.go: %v", err)
	}

	// Look for our exact string as a Go string literal in the source.
	needle := `"` + consensusFailureMsg + `"`
	if !strings.Contains(string(src), needle) {
		t.Fatalf(
			"CometBFT consensus/state.go no longer contains %s — "+
				"the consensusFailureMsg constant needs updating",
			needle,
		)
	}
}
