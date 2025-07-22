package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	cron "github.com/robfig/cron/v3"
	"github.com/xemle/shcron/internal/parser" // Ensure this path is correct for your repo
)

// CommandRunResult holds information about a single completed command execution.
type CommandRunResult struct {
	Timestamp time.Time // When the command was launched
	ExitCode  int       // Exit code of the command
	Err       error     // Error if the command failed to execute or returned non-zero
	RunNumber int       // The sequential number of this scheduled run
}

// ShcronApp holds all the configuration and state for the shcron application.
type ShcronApp struct {
	Pattern       string
	Command       string
	Args          []string
	ExitOnFailure bool
	OutputDir     string
	UntilDateStr  string
	Count         int
	ExitCodeMode  string
	MaxConcurrent int // Maximum number of parallel command executions

	untilTime        time.Time
	isCronPattern    bool
	intervalDuration time.Duration // For simple intervals
	cronScheduler    cron.Schedule // For cron patterns

	runMetrics *RunMetrics // To track execution results for final exit code

	// Concurrency control fields
	sem         chan struct{}         // Semaphore to limit concurrent goroutines
	resultsChan chan CommandRunResult // Channel to receive results from completed command goroutines
	wg          sync.WaitGroup        // To wait for all active command goroutines to finish on shutdown
	ctx         context.Context       // Master context for the application lifecycle
	cancel      context.CancelFunc    // Function to cancel the master context
}

// NewShcronApp creates and initializes a new ShcronApp.
func NewShcronApp(
	pattern string,
	command string,
	args []string,
	exitOnFailure bool,
	outputDir string,
	untilDateStr string,
	count int,
	exitCodeMode string,
	maxConcurrent int,
) *ShcronApp {
	ctx, cancel := context.WithCancel(context.Background())
	return &ShcronApp{
		Pattern:       pattern,
		Command:       command,
		Args:          args,
		ExitOnFailure: exitOnFailure,
		OutputDir:     outputDir,
		UntilDateStr:  untilDateStr,
		Count:         count,
		ExitCodeMode:  exitCodeMode,
		MaxConcurrent: maxConcurrent,
		runMetrics:    NewRunMetrics(),

		sem:         make(chan struct{}, maxConcurrent),
		resultsChan: make(chan CommandRunResult, maxConcurrent), // Buffered to prevent blocking if results aren't immediately processed
		ctx:         ctx,
		cancel:      cancel,
	}
}

// runCommand executes a single instance of the scheduled command.
// It runs in its own goroutine.
func (s *ShcronApp) runCommand(
	ctx context.Context, // Context to allow cancellation of the command
	runNum int,
	timestamp time.Time, // Launch timestamp
	outputDir string,
	command string,
	args []string,
	wg *sync.WaitGroup,
	resultsChan chan<- CommandRunResult, // send-only channel for results
) {
	defer wg.Done() // Decrement the waitgroup counter when this goroutine finishes.

	fmt.Printf("shcron: Running #%d (%s): %s %s\n", runNum, timestamp.Format(time.RFC3339), command, strings.Join(args, " "))

	cmdToRun := exec.CommandContext(ctx, command, args...)
	var runErr error
	var outputFile *os.File

	// Set process group ID to allow sending signals to child processes
	// This makes Ctrl+C (SIGTERM) work for the entire process tree.
	cmdToRun.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// NEW: Set SHCRON_RUN_ID environment variable for the command
	env := os.Environ() // Get current environment variables
	env = append(env, fmt.Sprintf("SHCRON_RUN_ID=%d", runNum))
	cmdToRun.Env = env // Set the command's environment

	// Determine output destination
	if outputDir != "" {
		// Create a unique output file path for each run
		outputFilePath := fmt.Sprintf("%s/%s_%s_%d.log", outputDir, timestamp.Format("20060102_150405_000"), strings.ReplaceAll(command, "/", "_"), runNum)
		var err error
		outputFile, err = os.Create(outputFilePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "shcron: Error creating output file %s for run #%d: %v\n", outputFilePath, runNum, err)
			// Fallback to os.Stdout/Stderr if file creation fails
			cmdToRun.Stdout = os.Stdout
			cmdToRun.Stderr = os.Stderr
		} else {
			cmdToRun.Stdout = outputFile
			cmdToRun.Stderr = outputFile
		}
	} else {
		// If no output directory, direct to standard output
		cmdToRun.Stdout = os.Stdout
		cmdToRun.Stderr = os.Stderr
	}

	// Execute the command
	runErr = cmdToRun.Run()

	// Close output file if it was opened
	if outputFile != nil {
		if err := outputFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "shcron: Error closing output file for run #%d: %v\n", runNum, err)
		}
	}

	exitCode := 0
	if runErr != nil {
		// Check if it's an ExitError (command returned non-zero)
		if exitError, ok := runErr.(*exec.ExitError); ok {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			}
		} else if ctx.Err() != nil {
			// If context was cancelled (e.g., via Ctrl+C) and command didn't exit cleanly
			// This means the command was likely killed by the context cancellation.
			fmt.Fprintf(os.Stderr, "shcron: Command #%d was terminated due to context cancellation.\n", runNum)
			exitCode = 1 // Or a specific code like 130 for SIGTERM, but 1 is generic non-zero.
		} else {
			// Other execution errors (e.g., command not found)
			fmt.Fprintf(os.Stderr, "shcron: Error executing command for run #%d: %v\n", runNum, runErr)
			exitCode = 127 // Standard for command not found, or similar non-exec errors
		}
	}

	// Only send result if the context hasn't been cancelled by the time we are done.
	select {
	case resultsChan <- CommandRunResult{
		Timestamp: timestamp,
		ExitCode:  exitCode,
		Err:       runErr,
		RunNumber: runNum,
	}:
		// Sent successfully
	case <-ctx.Done():
		// Context was cancelled, results channel might be closing or already closed.
		// Discard the result, as shutdown is in progress.
		fmt.Printf("shcron: Discarding result for run #%d as context is cancelled.\n", runNum)
	}
}

// Run executes the main shcron loop.
func (s *ShcronApp) Run() error {
	var err error

	// --- Initialization and Validation ---
	s.isCronPattern = parser.IsCronPattern(s.Pattern)

	if s.isCronPattern {
		cronParser := cron.NewParser(
			cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
		)
		s.cronScheduler, err = cronParser.Parse(s.Pattern)
		if err != nil {
			return fmt.Errorf("invalid cron pattern '%s': %w", s.Pattern, err)
		}
		fmt.Printf("shcron: Scheduling '%s %s' with cron pattern: '%s'\n", s.Command, strings.Join(s.Args, " "), s.Pattern)
	} else {
		s.intervalDuration, err = parser.ParseIntervalPattern(s.Pattern)
		if err != nil {
			return fmt.Errorf("invalid interval pattern '%s': %w", s.Pattern, err)
		}
		fmt.Printf("shcron: Scheduling '%s %s' every %s\n", s.Command, strings.Join(s.Args, " "), s.intervalDuration)
	}

	if s.UntilDateStr != "" {
		s.untilTime, err = ParseUntilDate(s.UntilDateStr)
		if err != nil {
			return fmt.Errorf("invalid --until date '%s': %w", s.UntilDateStr, err)
		}
		fmt.Printf("shcron: Will run until %s.\n", s.untilTime.Format(time.RFC3339))
	}

	if s.Count > 0 {
		fmt.Printf("shcron: Will run a maximum of %d times.\n", s.Count)
	}
	if s.ExitOnFailure {
		fmt.Println("shcron: Exiting on first command failure.")
	}
	if s.OutputDir != "" {
		fmt.Printf("shcron: Dumping output to '%s'\n", s.OutputDir)
		if _, err := os.Stat(s.OutputDir); os.IsNotExist(err) {
			if err := os.MkdirAll(s.OutputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory %s: %w", s.OutputDir, err)
			}
		}
	}
	if s.MaxConcurrent > 1 {
		fmt.Printf("shcron: Allowing up to %d concurrent command executions.\n", s.MaxConcurrent)
	} else {
		fmt.Println("shcron: Running commands sequentially.")
	}

	// --- Signal Handling ---
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("shcron: Press Ctrl+C to abort.\n")

	// Goroutine to handle results from completed commands
	go func() {
		for res := range s.resultsChan {
			s.runMetrics.RecordCompletedRun(res)
			if s.ExitOnFailure && res.ExitCode != 0 {
				fmt.Fprintf(os.Stderr, "shcron: Command #%d exited with non-zero code: %d. Terminating due to --exit-on-failure.\n", res.RunNumber, res.ExitCode)
				s.cancel() // Signal main scheduler to stop
				return     // Exit this goroutine
			}
		}
	}()

	// --- Main Execution Loop ---
	for {
		// --- Check for termination signals or conditions ---
		select {
		case sig := <-sigChan: // User signal (e.g., Ctrl+C)
			fmt.Printf("\nshcron: Received signal %s. Initiating graceful shutdown.\n", sig)
			s.cancel() // Cancel the context to signal all parts of the app to stop
			// Fall through to the ctx.Done() case below
		case <-s.ctx.Done(): // Context was cancelled (either by signal or --exit-on-failure)
			fmt.Println("\nshcron: Termination signal received. Waiting for active tasks to finish...")
			s.wg.Wait()          // Wait for all outstanding command goroutines to complete
			close(s.resultsChan) // Close results channel AFTER all goroutines have finished
			// Determine final exit code. `finishedNormally` is true if context was NOT cancelled
			HandleExit(s.ExitCodeMode, s.runMetrics, s.ctx.Err() != context.Canceled)
			return nil // Exits the Run() function
		default:
			// No signal/cancel, continue with scheduling logic
		}

		// --- Check maximum run count and until date conditions ---
		s.runMetrics.mu.Lock()
		currentTotalRuns := s.runMetrics.TotalRuns
		s.runMetrics.mu.Unlock()

		if s.Count > 0 && currentTotalRuns >= s.Count {
			fmt.Printf("shcron: Maximum scheduled run count (%d) reached. Initiating shutdown.\n", s.Count)
			s.cancel() // Signal termination
			continue   // Go back to select to handle ctx.Done()
		}

		if !s.untilTime.IsZero() && time.Now().After(s.untilTime) {
			fmt.Printf("shcron: Until date (%s) reached. Initiating shutdown.\n", s.untilTime.Format(time.RFC3339))
			s.cancel() // Signal termination
			continue   // Go back to select to handle ctx.Done()
		}

		// Calculate sleep duration for the next scheduled run
		var sleepDuration time.Duration
		scheduleNow := false

		if s.isCronPattern {
			now := time.Now()
			nextRun := s.cronScheduler.Next(now)
			if nextRun.IsZero() {
				fmt.Fprintf(os.Stderr, "shcron: Cron pattern '%s' did not produce a future time. Exiting.\n", s.Pattern)
				s.cancel()
				continue
			}
			sleepDuration = nextRun.Sub(now)
			fmt.Printf("shcron: Next run scheduled for %s (sleep for %s)\n", nextRun.Format(time.RFC3339), sleepDuration.Round(time.Second))
		} else {
			// For interval patterns, execute immediately on first run
			if currentTotalRuns == 0 { // This is the very first loop iteration for an interval
				scheduleNow = true
				fmt.Printf("shcron: Launching first run immediately (interval mode).\n")
			} else {
				sleepDuration = s.intervalDuration
				fmt.Printf("shcron: Next run scheduled in %s\n", sleepDuration.Round(time.Second))
			}
		}

		// Wait for the next scheduled time OR for a termination signal, unless scheduling immediately.
		if !scheduleNow {
			select {
			case <-time.After(sleepDuration):
				// Time to schedule a new run.
			case <-s.ctx.Done():
				fmt.Println("shcron: Aborting schedule due to termination signal received during sleep.")
				continue // Go back to select to handle ctx.Done() and clean up.
			}
		}

		// Try to acquire a concurrency slot. This will block if MaxConcurrent limit is reached.
		select {
		case <-s.ctx.Done():
			fmt.Println("shcron: Not scheduling new task as termination signal received while waiting for slot.")
			continue // Go back to select to handle ctx.Done() and clean up.
		case s.sem <- struct{}{}: // Acquire a slot in the semaphore
			// Slot acquired, proceed to launch command
		}

		// Increment the total scheduled runs count.
		s.runMetrics.mu.Lock()
		s.runMetrics.TotalRuns++
		currentRunNumber := s.runMetrics.TotalRuns
		s.runMetrics.mu.Unlock()

		// Launch the command in a new goroutine.
		s.wg.Add(1) // Increment waitgroup counter.
		go func(runNum int, ts time.Time) {
			defer func() { <-s.sem }() // Release the semaphore slot when this goroutine finishes.
			s.runCommand(s.ctx, runNum, ts, s.OutputDir, s.Command, s.Args, &s.wg, s.resultsChan)
		}(currentRunNumber, time.Now()) // Pass a fresh timestamp for this specific run.
	}
}
