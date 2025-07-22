package app

import (
	"os"
	"sync" // NEW: For mutex
)

// RunMetrics stores the state needed to determine the final exit code.
// It's designed to be concurrency-safe with a mutex.
type RunMetrics struct {
	mu sync.Mutex // Mutex to protect access to fields

	TotalRuns     int // Total number of runs *scheduled*
	CompletedRuns int // Total number of runs *that have finished*

	FirstScheduledRunExitCode int // Exit code of the first run that was *scheduled* and *completed*
	LastCompletedRunExitCode  int // Exit code of the last run that *completed*

	FirstErrorExitCode int  // Exit code of the first *completed* run with a non-zero exit code
	LastErrorExitCode  int  // Exit code of the last *completed* run with a non-zero exit code
	HadError           bool // True if any completed run had a non-zero exit code
}

// NewRunMetrics initializes a new RunMetrics struct.
func NewRunMetrics() *RunMetrics {
	return &RunMetrics{}
}

// RecordCompletedRun updates metrics after a single command execution has finished.
func (m *RunMetrics) RecordCompletedRun(result CommandRunResult) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CompletedRuns++
	m.LastCompletedRunExitCode = result.ExitCode

	if m.CompletedRuns == 1 { // If this is the very first run that completed
		m.FirstScheduledRunExitCode = result.ExitCode
	}

	if result.ExitCode != 0 {
		if !m.HadError {
			m.FirstErrorExitCode = result.ExitCode
			m.HadError = true
		}
		m.LastErrorExitCode = result.ExitCode
	}
}

// HandleExit determines and sets the final shcron exit code.
// finishedNormally is true if shcron completed due to --count or --until, not a signal or --exit-on-failure.
func HandleExit(mode string, metrics *RunMetrics, finishedNormally bool) {
	metrics.mu.Lock() // Lock to ensure consistent metrics for final decision
	defer metrics.mu.Unlock()

	finalShcronExitCode := 0

	if metrics.TotalRuns == 0 && !finishedNormally {
		// If no runs were even scheduled and it's not a normal finish (e.g., immediate Ctrl+C)
		os.Exit(0) // Exit cleanly
	}
	if metrics.CompletedRuns == 0 && metrics.TotalRuns > 0 && !finishedNormally {
		// If runs were scheduled but none completed (e.g., immediate abort during long first run)
		// We can't use metrics from completed runs, so exit cleanly unless specific error occurred.
		os.Exit(0)
	}

	if finishedNormally { // Loop finished because count/until was reached
		switch mode {
		case "first-run":
			finalShcronExitCode = metrics.FirstScheduledRunExitCode
		case "last-run":
			finalShcronExitCode = metrics.LastCompletedRunExitCode
		case "first-error":
			finalShcronExitCode = metrics.FirstErrorExitCode
		case "last-error":
			finalShcronExitCode = metrics.LastErrorExitCode
		default: // default or any unrecognized mode
			finalShcronExitCode = 0
		}
	} else { // Aborted by user (Ctrl+C) or --exit-on-failure
		switch mode {
		case "first-error":
			if metrics.HadError {
				finalShcronExitCode = metrics.FirstErrorExitCode
			} else {
				finalShcronExitCode = 0 // No errors occurred before abort
			}
		case "last-error":
			if metrics.HadError {
				finalShcronExitCode = metrics.LastErrorExitCode
			} else {
				finalShcronExitCode = 0 // No errors occurred before abort
			}
		default:
			finalShcronExitCode = 0 // Default for user abort/non-specific termination
		}
	}
	os.Exit(finalShcronExitCode)
}
