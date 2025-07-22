package app

import (
	"os"
)

// RunMetrics stores the state needed to determine the final exit code.
type RunMetrics struct {
	TotalRuns          int
	FirstRunExitCode   int
	LastRunExitCode    int
	FirstErrorExitCode int
	LastErrorExitCode  int
	HadError           bool
}

// NewRunMetrics initializes a new RunMetrics struct.
func NewRunMetrics() *RunMetrics {
	return &RunMetrics{}
}

// RecordRun updates metrics after each command execution.
func (m *RunMetrics) RecordRun(exitCode int) {
	if m.TotalRuns == 1 {
		m.FirstRunExitCode = exitCode
	}
	m.LastRunExitCode = exitCode

	if exitCode != 0 {
		if !m.HadError {
			m.FirstErrorExitCode = exitCode
			m.HadError = true
		}
		m.LastErrorExitCode = exitCode
	}
}

// HandleExit determines and sets the final shcron exit code.
func HandleExit(mode string, metrics *RunMetrics, finishedNormally bool) {
	finalShcronExitCode := 0

	if finishedNormally { // Loop finished because count/until was reached
		switch mode {
		case "first-run":
			finalShcronExitCode = metrics.FirstRunExitCode
		case "last-run":
			finalShcronExitCode = metrics.LastRunExitCode
		case "first-error":
			finalShcronExitCode = metrics.FirstErrorExitCode
		case "last-error":
			finalShcronExitCode = metrics.LastErrorExitCode
		default: // default or "0"
			finalShcronExitCode = 0
		}
	} else { // Aborted by user (Ctrl+C)
		switch mode {
		case "first-error":
			if metrics.HadError {
				finalShcronExitCode = metrics.FirstErrorExitCode
			} else {
				finalShcronExitCode = 0
			}
		case "last-error":
			if metrics.HadError {
				finalShcronExitCode = metrics.LastErrorExitCode
			} else {
				finalShcronExitCode = 0
			}
		default:
			finalShcronExitCode = 0 // Default for user abort
		}
	}
	os.Exit(finalShcronExitCode)
}
