package app

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	cron "github.com/robfig/cron/v3"
	"github.com/xemle/shcron/internal/parser"
)

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

	untilTime        time.Time
	isCronPattern    bool
	intervalDuration time.Duration // For simple intervals
	cronScheduler    cron.Schedule // For cron patterns

	runMetrics *RunMetrics // To track execution results for exit code
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
) *ShcronApp {
	return &ShcronApp{
		Pattern:       pattern,
		Command:       command,
		Args:          args,
		ExitOnFailure: exitOnFailure,
		OutputDir:     outputDir,
		UntilDateStr:  untilDateStr,
		Count:         count,
		ExitCodeMode:  exitCodeMode,
		runMetrics:    NewRunMetrics(),
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
		s.untilTime, err = ParseUntilDate(s.UntilDateStr) // Use the ParseUntilDate from this package
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

	// Handle graceful shutdown (Ctrl+C)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("shcron: Press Ctrl+C to abort.\n")

	// --- Initial sleep for cron patterns to align to the next scheduled time ---
	if s.isCronPattern {
		now := time.Now()
		nextRun := s.cronScheduler.Next(now)
		if nextRun.IsZero() {
			return fmt.Errorf("cron pattern '%s' did not produce a future time from %s", s.Pattern, now.Format(time.RFC3339))
		}
		sleepDuration := nextRun.Sub(now)
		fmt.Printf("shcron: Next run at %s (sleeping for %s)\n", nextRun.Format(time.RFC3339), sleepDuration.Round(time.Second))
		select {
		case <-time.After(sleepDuration):
			// Proceed
		case <-sigChan:
			fmt.Println("\nshcron: Aborting during initial sleep.")
			HandleExit(s.ExitCodeMode, s.runMetrics, false) // Aborted
		}
	}

	// --- Main Execution Loop ---
	for {
		// Check termination conditions before running current task
		if s.Count > 0 && s.runMetrics.TotalRuns >= s.Count {
			fmt.Printf("shcron: Maximum run count (%d) reached.\n", s.Count)
			HandleExit(s.ExitCodeMode, s.runMetrics, true)
		}

		if !s.untilTime.IsZero() && time.Now().After(s.untilTime) {
			fmt.Printf("shcron: Until date (%s) reached.\n", s.untilTime.Format(time.RFC3339))
			HandleExit(s.ExitCodeMode, s.runMetrics, true)
		}

		s.runMetrics.TotalRuns++
		timestamp := time.Now().Format("20060102_150405_000") // YYYYMMDD_HHMMSS_XXX
		fmt.Printf("shcron: Running #%d at %s\n", s.runMetrics.TotalRuns, time.Now().Format(time.RFC3339))

		cmdToRun := exec.Command(s.Command, s.Args...)
		var runErr error

		if s.OutputDir != "" {
			outputFile := fmt.Sprintf("%s/%s_%s.log", s.OutputDir, timestamp, strings.ReplaceAll(s.Command, "/", "_"))
			f, err := os.Create(outputFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "shcron: Error creating output file %s: %v\n", outputFile, err)
				// Decide how to handle this critical error:
				// Option 1: Treat as a command failure (if it prevents command from running meaningfully)
				// Option 2: Continue, but log to shcron's stderr directly for this run (current behavior)
				// For now, we'll continue with os.Stdout/Stderr as fallback, but log the error.
				cmdToRun.Stdout = os.Stdout
				cmdToRun.Stderr = os.Stderr
			} else {
				cmdToRun.Stdout = f
				cmdToRun.Stderr = f
			}
		} else {
			cmdToRun.Stdout = os.Stdout
			cmdToRun.Stderr = os.Stderr
		}

		runErr = cmdToRun.Run()

		exitCode := 0
		if runErr != nil {
			if exitError, ok := runErr.(*exec.ExitError); ok {
				if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
					exitCode = status.ExitStatus()
				}
			} else {
				fmt.Fprintf(os.Stderr, "shcron: Error executing command: %v\n", runErr)
				exitCode = 127 // Standard for command not found, or similar non-exec errors
			}
		}

		s.runMetrics.RecordRun(exitCode)

		if exitCode != 0 {
			fmt.Fprintf(os.Stderr, "shcron: Command exited with non-zero code: %d\n", exitCode)
			if s.ExitOnFailure {
				fmt.Println("shcron: Exiting due to --exit-on-failure.")
				os.Exit(exitCode) // Exit with the command's exit code
			}
		}

		// Calculate sleep duration for the next cycle
		var sleepDuration time.Duration
		if s.isCronPattern {
			now := time.Now()
			nextRun := s.cronScheduler.Next(now)
			if nextRun.IsZero() {
				fmt.Fprintf(os.Stderr, "shcron: Cron pattern '%s' did not produce a future time. Exiting.\n", s.Pattern)
				HandleExit(s.ExitCodeMode, s.runMetrics, true) // Indicate an internal error implicitly
			}
			sleepDuration = nextRun.Sub(now)
			fmt.Printf("shcron: Next run at %s (sleeping for %s)\n", nextRun.Format(time.RFC3339), sleepDuration.Round(time.Second))
		} else {
			sleepDuration = s.intervalDuration
		}

		select {
		case <-time.After(sleepDuration):
			// Proceed to next iteration
		case <-sigChan:
			fmt.Println("\nshcron: Aborting by user request.")
			HandleExit(s.ExitCodeMode, s.runMetrics, false) // Aborted
		}
	}
}
