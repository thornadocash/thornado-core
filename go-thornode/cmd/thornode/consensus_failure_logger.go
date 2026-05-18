package main

import (
	"os"

	"cosmossdk.io/log"
)

// consensusFailureMsg is the exact log message emitted by CometBFT's
// receiveRoutine when it recovers from a consensus panic.
const consensusFailureMsg = "CONSENSUS FAILURE!!!"

// ConsensusFailureLogger wraps a cosmossdk.io/log.Logger and monitors for
// CometBFT consensus failure log messages. When CometBFT's consensus
// receiveRoutine catches a panic, it logs "CONSENSUS FAILURE!!!" and silently
// returns — leaving the process alive but unable to participate in consensus.
// This wrapper detects that message and terminates the process so that
// container orchestration can restart the node.
type ConsensusFailureLogger struct {
	log.Logger
}

// NewConsensusFailureLogger returns a logger that exits the process when a
// CometBFT consensus failure is detected.
func NewConsensusFailureLogger(inner log.Logger) ConsensusFailureLogger {
	return ConsensusFailureLogger{Logger: inner}
}

func (l ConsensusFailureLogger) Error(msg string, keyvals ...interface{}) {
	// Always log the message first so operators see the full error.
	l.Logger.Error(msg, keyvals...)

	if msg == consensusFailureMsg {
		// The consensus goroutine is dead. Other goroutines (gRPC, API, P2P)
		// keep the process alive but the node can no longer produce or validate
		// blocks. Exit immediately so the container restarts.
		os.Exit(1)
	}
}

func (l ConsensusFailureLogger) With(keyvals ...interface{}) log.Logger {
	return NewConsensusFailureLogger(l.Logger.With(keyvals...))
}

func (l ConsensusFailureLogger) Impl() interface{} {
	return l.Logger.Impl()
}
