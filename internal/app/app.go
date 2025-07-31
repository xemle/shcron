package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal" // Make sure signal is imported
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/xemle/shcron/internal/schedule"
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
	Scheduler schedule.ScheduleStrategy
	Command   string
	Args      []string

	ExitOnFailure bool
	OutputDir     string
	UntilDateStr  string
	Count         int
	ExitCodeMode  string
	MaxConcurrent int // Maximum number of parallel command executions

	untilTime  time.Time
	runMetrics *RunMetrics // To track execution results for final exit code

	// Concurrency control fields
	sem         chan struct{}         // Semaphore to limit concurrent goroutines
	resultsChan chan CommandRunResult // Channel to receive results from completed command goroutines
	wg          sync.WaitGroup        // To wait for all active command goroutines to finish on shutdown
	ctx         context.Context       // Master context for the application lifecycle
	cancel      context.CancelFunc    // Function to cancel the master context

	// Extracted interfaces for testability
	commandExecutor CommandExecutor
	fileManager     FileManager
	clock           Clock
}

// NewShcronApp creates and initializes a new ShcronApp.
// It uses default implementations for CommandExecutor, FileManager, and Clock.
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
) (*ShcronApp, error) {
	scheduler, err := schedule.CreateScheduler(pattern)
	if err != nil {
		return nil, err
	}

	clock := SystemClock{}
	untilTime := time.Time{}
	if untilDateStr != "" {
		untilTime, err = schedule.ParseUntilDate(untilDateStr, clock.Now())
		if err != nil {
			return nil, fmt.Errorf("invalid --until date '%s': %w", untilTime, err)
		}
		fmt.Printf("shcron: Will run until %s.\n", untilTime.Format(time.RFC3339))
	} else {
		fmt.Printf("shcron: Will run forever until abort via CTRL+c.\n")
	}

	return NewShcronAppWithDependencies(
		command,
		args,

		exitOnFailure,
		outputDir,
		count,
		exitCodeMode,
		maxConcurrent,

		scheduler,
		untilTime,
		&OsCommandExecutor{},
		&OsFileManager{},
		&clock,
	)
}

// NewShcronAppWithDependencies creates a new ShcronApp with custom dependencies (for testing).
func NewShcronAppWithDependencies(
	command string,
	args []string,

	exitOnFailure bool,
	outputDir string,
	count int,
	exitCodeMode string,
	maxConcurrent int,

	scheduler schedule.ScheduleStrategy,
	untilTime time.Time,
	commandExecutor CommandExecutor,
	fileManager FileManager,
	clock Clock,
) (*ShcronApp, error) {
	ctx, cancel := context.WithCancel(context.Background())

	app := &ShcronApp{
		Command: command,
		Args:    args,

		ExitOnFailure: exitOnFailure,
		OutputDir:     outputDir,
		Count:         count,
		ExitCodeMode:  exitCodeMode,
		MaxConcurrent: maxConcurrent,

		Scheduler:       scheduler,
		untilTime:       untilTime,
		commandExecutor: commandExecutor,
		fileManager:     fileManager,
		clock:           clock,

		runMetrics:  NewRunMetrics(),
		sem:         make(chan struct{}, maxConcurrent),
		resultsChan: make(chan CommandRunResult, maxConcurrent),
		ctx:         ctx,
		cancel:      cancel,
	}

	err := app.initOptions()
	if err != nil {
		return nil, err
	}

	return app, nil
}

// runCommand executes a single instance of the scheduled command.
// It runs in its own goroutine.
func (s *ShcronApp) runCommand(
	runNum int,
	timestamp time.Time, // Launch timestamp
) {
	defer s.wg.Done() // Decrement the waitgroup counter when this goroutine finishes.

	fmt.Printf("shcron: Running #%d (%s): %s %s\n", runNum, timestamp.Format(time.RFC3339), s.Command, strings.Join(s.Args, " "))

	cmdToRun := s.commandExecutor.CommandContext(s.ctx, s.Command, s.Args...)
	var runErr error
	var outputFile io.WriteCloser // Use io.WriteCloser for the file interface

	// Set SHCRON_RUN_ID environment variable for the command
	env := s.commandExecutor.GetEnviron() // Use injected method
	env = append(env, fmt.Sprintf("SHCRON_RUN_ID=%d", runNum))
	cmdToRun.SetEnv(env) // Use injected method

	// Determine output destination
	if s.OutputDir != "" {
		outputFilePath := fmt.Sprintf("%s/%s_%s_%d.log", s.OutputDir, timestamp.Format("20060102_150405_000"), strings.ReplaceAll(s.Command, "/", "_"), runNum)
		var err error
		outputFile, err = s.fileManager.Create(outputFilePath) // Use injected method
		if err != nil {
			fmt.Fprintf(os.Stderr, "shcron: Error creating output file %s for run #%d: %v\n", outputFilePath, runNum, err)
			// Fallback to os.Stdout/Stderr if file creation fails
			cmdToRun.SetStdout(os.Stdout)
			cmdToRun.SetStderr(os.Stderr)
		} else {
			cmdToRun.SetStdout(outputFile)
			cmdToRun.SetStderr(outputFile)
		}
	} else {
		cmdToRun.SetStdout(os.Stdout)
		cmdToRun.SetStderr(os.Stderr)
	}

	runErr = cmdToRun.Run()

	if outputFile != nil {
		if err := outputFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "shcron: Error closing output file for run #%d: %v\n", runNum, err)
		}
	}

	exitCode := 0
	if runErr != nil {
		if exitError, ok := runErr.(*exec.ExitError); ok {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			}
		} else if s.ctx.Err() != nil {
			// If runErr is not an ExitError but context was cancelled, it means
			// the command was likely killed/terminated by us.
			fmt.Fprintf(os.Stderr, "shcron: Command #%d was terminated due to context cancellation.\n", runNum)
			exitCode = 1
		} else {
			fmt.Fprintf(os.Stderr, "shcron: Error executing command for run #%d: %v\n", runNum, runErr)
			exitCode = 127
		}
	}

	select {
	case s.resultsChan <- CommandRunResult{
		Timestamp: timestamp,
		ExitCode:  exitCode,
		Err:       runErr,
		RunNumber: runNum,
	}:
		// Sent successfully
	case <-s.ctx.Done():
		fmt.Printf("shcron: Discarding result for run #%d as context is cancelled.\n", runNum)
	}
}

// Run executes the main shcron loop.
func (s *ShcronApp) Run() error {
	// --- Signal Handling Setup ---
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM) // Capture Ctrl+C and termination signals

	fmt.Printf("shcron: Press Ctrl+C to abort.\n")

	// Goroutine to handle results from completed commands
	go func() {
		for res := range s.resultsChan {
			s.runMetrics.RecordCompletedRun(res)
			if s.ExitOnFailure && res.ExitCode != 0 {
				fmt.Fprintf(os.Stderr, "shcron: Command #%d exited with non-zero code: %d. Terminating due to --exit-on-failure.\n", res.RunNumber, res.ExitCode)
				s.cancel() // THIS WILL TRIGGER THE MAIN LOOP'S CANCELLATION
				return
			}
		}
	}()

	lastRunTime := s.clock.Now()

	// --- Main Execution Loop ---
Loop:
	for {
		// --- Immediate Termination Checks ---
		// Check for context cancellation from external signals, count limit, until date, or exit-on-failure
		select {
		case sig := <-sigChan:
			fmt.Printf("\nshcron: Received signal %s. Initiating graceful shutdown.\n", sig)
			s.cancel() // Cancel the master context
		case <-s.ctx.Done():
			// Context was cancelled, so break out of the main loop.
			// This covers cancellation from `s.cancel()` called by `sigChan`, `ExitOnFailure`, `Count`, `UntilDate`.
			break Loop
		default:
			// No immediate signal or cancellation, continue with scheduling logic.
		}

		// If context is done, break the loop and proceed to cleanup.
		if s.ctx.Err() != nil {
			break
		}

		// --- Scheduling Checks ---
		currentTotalRuns := s.runMetrics.GetTotalRuns()

		if s.Count > 0 && currentTotalRuns >= s.Count {
			s.wg.Wait()
			fmt.Printf("shcron: Maximum scheduled run count (%d) reached. Initiating shutdown.\n", s.Count)
			s.cancel()
			continue // Re-enter loop to hit `s.ctx.Done()`
		}

		if !s.untilTime.IsZero() && lastRunTime.After(s.untilTime) {
			fmt.Printf("shcron: Until date (%s) reached. Initiating shutdown.\n", s.untilTime.Format(time.RFC3339))
			s.cancel()
			continue // Re-enter loop to hit `s.ctx.Done()`
		}

		// Determine the next scheduled time
		nextScheduledTime := lastRunTime
		if currentTotalRuns == 0 && !s.Scheduler.IsCron() {
			fmt.Printf("shcron: Launching first run immediately (interval mode).\n")
		} else {
			nextScheduledTime = s.Scheduler.Next(lastRunTime)
			if nextScheduledTime.IsZero() {
				fmt.Fprintf(os.Stderr, "shcron: Scheduler did not produce a future time. Exiting.\n")
				s.cancel() // No more runs possible, cancel to exit
				continue   // Re-enter loop to hit `s.ctx.Done()`
			}
			fmt.Printf("shcron: Next run scheduled for %s (sleep for %s)\n", nextScheduledTime.Format(time.RFC3339), nextScheduledTime.Sub(lastRunTime).Round(time.Second))
		}

		// Calculate sleep duration until next scheduled time
		sleepDuration := nextScheduledTime.Sub(lastRunTime)
		if sleepDuration < 0 {
			sleepDuration = 0 // Should run immediately if nextScheduledTime is in the past
		}

		// --- Wait for Next Schedule Time OR Cancellation ---
		// This select handles the waiting. It will be interrupted by signals/context cancellation.
		if sleepDuration > 0 { // Only sleep if there's a duration
			select {
			case <-s.clock.After(sleepDuration):
				// Time for next run. Continue to acquire semaphore and launch.
			case sig := <-sigChan:
				fmt.Printf("\nshcron: Received signal %s. Initiating graceful shutdown.\n", sig)
				s.cancel()
				continue // Re-enter loop to hit `s.ctx.Done()`
			case <-s.ctx.Done():
				// Context was cancelled during sleep, so re-enter loop to handle `s.ctx.Done()`
				continue
			}
		}

		// If context is done after waiting, break the loop.
		if s.ctx.Err() != nil {
			break
		}

		// --- Acquire Semaphore Slot OR Cancellation ---
		select {
		case s.sem <- struct{}{}:
			// Slot acquired, proceed to launch
		case sig := <-sigChan: // Check for signals again while waiting for semaphore
			fmt.Printf("\nshcron: Received signal %s. Initiating graceful shutdown.\n", sig)
			s.cancel()
			continue // Re-enter loop to hit `s.ctx.Done()`
		case <-s.ctx.Done(): // Check for context cancellation again
			fmt.Println("shcron: Not scheduling new task as termination signal received while waiting for slot.")
			continue // Re-enter loop to hit `s.ctx.Done()`
		}

		// If context is done after acquiring semaphore, break the loop.
		if s.ctx.Err() != nil {
			break
		}

		// --- Launch Command ---
		s.runMetrics.IncRuns()
		s.wg.Add(1)
		go func(runNum int, time time.Time) {
			defer func() { <-s.sem }() // Release semaphore slot when goroutine finishes
			s.runCommand(runNum, time)
		}(currentTotalRuns, nextScheduledTime)

		lastRunTime = nextScheduledTime
	}

	// --- Cleanup after main loop exits ---
	fmt.Println("shcron: Waiting for active tasks to finish...")
	s.wg.Wait()          // Wait for all command goroutines to finish
	close(s.resultsChan) // Close the results channel after all producers (runCommand) are done

	// Handle the final exit code based on metrics and whether it was a clean exit or forced termination
	HandleExit(s.ExitCodeMode, s.runMetrics, s.ctx.Err() != context.Canceled)
	return nil
}

func (s *ShcronApp) initOptions() error {
	if s.Count > 0 {
		fmt.Printf("shcron: Will run a maximum of %d times.\n", s.Count)
	}
	if s.ExitOnFailure {
		fmt.Println("shcron: Exiting on first command failure.")
	}
	if s.OutputDir != "" {
		fmt.Printf("shcron: Dumping output to '%s'\n", s.OutputDir)
		// Check if the directory exists
		_, statErr := s.fileManager.Stat(s.OutputDir) // Call Stat once and capture the error

		// If the directory does NOT exist (os.IsNotExist(statErr))
		// OR if there's any other error besides "not exist" that prevents us from using it
		if statErr != nil && os.IsNotExist(statErr) {
			// Directory does not exist, so try to create it
			if err := s.fileManager.MkdirAll(s.OutputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory %s: %w", s.OutputDir, err)
			}
		} else if statErr != nil {
			// It's not a "does not exist" error, but some other problem
			// (e.g., permissions, not a directory, etc.). We should report it.
			return fmt.Errorf("failed to access or verify output directory %s: %w", s.OutputDir, statErr)
		}
		// If statErr is nil, the directory already exists and is accessible, so no action needed.
	}
	if s.MaxConcurrent > 1 {
		fmt.Printf("shcron: Allowing up to %d concurrent command executions.\n", s.MaxConcurrent)
	} else {
		fmt.Println("shcron: Running commands sequentially.")
	}
	return nil
}
