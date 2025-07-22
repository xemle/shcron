package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/xemle/shcron/internal/app"
)

var (
	exitOnFailure bool
	outputDir     string
	untilDateStr  string
	count         int
	exitCodeMode  string
	maxConcurrent int // NEW: Flag for maximum concurrent executions
)

// version will be set by the Go linker during build for releases
var version string = "dev"

func main() {
	// Define flags
	flag.BoolVar(&exitOnFailure, "exit-on-failure", false, "Exit immediately if the command returns a non-zero exit code.")
	flag.BoolVar(&exitOnFailure, "e", false, "Exit immediately if the command returns a non-zero exit code (shorthand).")

	flag.StringVar(&outputDir, "output-dir", "", "Directory to dump command output (one file per run).")
	flag.StringVar(&outputDir, "o", "", "Directory to dump command output (one file per run) (shorthand).")

	flag.StringVar(&untilDateStr, "until", "", "Maximum date/time until the task should repeat (e.g., '2025-12-31', 'next day', 'in 3 hours').")
	flag.StringVar(&untilDateStr, "u", "", "Maximum date/time until the task should repeat (shorthand).")

	flag.IntVar(&count, "count", 0, "Maximum number of times the task should be repeated (0 for infinite).")
	flag.IntVar(&count, "c", 0, "Maximum number of times the task should be repeated (0 for infinite) (shorthand).")

	flag.StringVar(&exitCodeMode, "exit-code", "default", "Defines shcron's exit code on termination. Options: first-run, last-run, first-error, last-error, default.")
	flag.StringVar(&exitCodeMode, "x", "default", "Defines shcron's exit code on termination (shorthand).")

	flag.IntVar(&maxConcurrent, "max-concurrent", 1, "Maximum number of concurrent command executions. 1 for sequential.")
	flag.IntVar(&maxConcurrent, "j", 1, "Maximum number of concurrent command executions (shorthand).")

	// Special flag for version
	versionFlag := flag.Bool("version", false, "Print shcron version and exit.")

	// Set custom usage message
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `shcron: A flexible command-line tool for periodic execution, similar to cron but for temporary tasks.
It supports both simple interval patterns and full cron expressions for precise scheduling, and can run tasks in parallel.

Usage: %s [options] "<pattern>" <command> [args...]

Pattern Examples:
  Intervals:
    "5s"  : Every 5 seconds
    "1m"  : Every 1 minute
    "30m" : Every 30 minutes
    "2h"  : Every 2 hours
    "1d"  : Every 1 day

  Cron Expressions (5 fields: Minute Hour DayOfMonth Month DayOfWeek):
    "* * * * *"   : Every minute
    "0 * * * *"   : Every hour, at minute 0
    "0 9 * * 1-5" : Every weekday (Mon-Fri) at 9:00 AM
    "0 0 1 * *"   : On the 1st of every month at midnight

Date/Time Format for --until:
  Uses flexible parsing: "YYYY-MM-DD", "YYYY-MM-DD HH:MM", "next day", "in 3 hours", etc.

Options:
`, os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse() // Parse the flags

	if *versionFlag {
		fmt.Printf("shcron version %s\n", version)
		os.Exit(0)
	}

	// Positional arguments start after flags
	args := flag.Args()

	if len(args) < 2 {
		flag.Usage()
		os.Exit(1)
	}

	if maxConcurrent < 1 {
		fmt.Fprintf(os.Stderr, "Error: --max-concurrent must be at least 1.\n")
		os.Exit(1)
	}

	patternStr := args[0]
	command := args[1]
	cmdArgs := args[2:]

	// Create and run the ShcronApp
	app := app.NewShcronApp(
		patternStr,
		command,
		cmdArgs,
		exitOnFailure,
		outputDir,
		untilDateStr,
		count,
		exitCodeMode,
		maxConcurrent, // Pass new flag
	)

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	// Note: app.Run() now explicitly exits via HandleExit, so this line should generally not be reached.
	// It's here primarily for cases where Run() might return an internal error before HandleExit.
}
