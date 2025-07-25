package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"               // Using pflag for better CLI experience
	"github.com/xemle/shcron/internal/app" // Make sure this import path is correct for your project
)

var version string = "dev"

func main() {
	count := pflag.IntP("count", "c", 0, "Stop scheduling after N runs (0 for unlimited)")
	exitCodeMode := pflag.StringP("exit-code-mode", "x", "last", "Mode for shcron's exit code: 'last', 'any-fail', 'all-fail'")
	exitOnFailure := pflag.BoolP("exit-on-failure", "e", false, "Exit immediately if a command fails (returns non-zero exit code)")
	maxConcurrent := pflag.IntP("max-concurrent", "j", 1, "Maximum number of concurrent command executions")
	outputDir := pflag.StringP("output-dir", "o", "", "Directory to dump command stdout/stderr (e.g., '/var/log/shcron')")
	untilDateStr := pflag.StringP("until", "u", "", "Stop scheduling at a specific date/time (e.g., '2025-12-31 23:59:59')")
	versionFlag := pflag.Bool("version", false, "Print shcron version and exit.")

	pflag.Usage = func() {
		fmt.Fprintf(pflag.CommandLine.Output(), `shcron: A flexible command-line tool for periodic execution, similar to cron but for temporary tasks.
It supports both simple interval patterns and full cron expressions for precise scheduling, and can run tasks in parallel.

Usage: %s [options] "<pattern>" <command> [args...]

Pattern Examples:
  Intervals:
    "5s"  : Every 5 seconds
    "1m"  : Every 1 minute
    "30m" : Every 30 minutes
    "2h"  : Every 2 hours

  Cron Expressions (6 fields: Second Minute Hour DayOfMonth Month DayOfWeek):
    "* * * * * *"   : Every second
    "*/10 * * * * *": Every 10 seconds
    "0 * * * * *"   : Every minute
    "0 5 * * * *"   : Every hour, at minute 5
    "0 0 9 * * 1-5" : Every weekday (Mon-Fri) at 9:00 AM
    "0 0 0 1 * *"   : On the 1st of every month at midnight

Date/Time Format for --until:
  Uses parsing: "YYYY-MM-DD", "YYYY-MM-DD HH:MM"

Options:
`, os.Args[0])
		pflag.PrintDefaults()
	}

	pflag.Parse()

	if *versionFlag {
		fmt.Printf("shcron version %s\n", version)
		os.Exit(0)
	}

	// Positional arguments start after flags
	args := pflag.Args()

	if len(args) < 2 {
		fmt.Println("Error: pattern and command are required.")
		pflag.Usage()
		os.Exit(1)
	}

	pattern := args[0]
	command := args[1]
	args = args[2:]

	validExitCodeModes := map[string]bool{"last": true, "any-fail": true, "all-fail": true}
	if !validExitCodeModes[strings.ToLower(*exitCodeMode)] {
		fmt.Fprintf(os.Stderr, "Error: Invalid --exit-code-mode '%s'. Must be 'last', 'any-fail', or 'all-fail'.\n", *exitCodeMode)
		os.Exit(1)
	}

	if *maxConcurrent < 1 {
		fmt.Fprintf(os.Stderr, "Error: --max-concurrent must be at least 1.\n")
		os.Exit(1)
	}

	appInstance, err := app.NewShcronApp(
		pattern,
		command,
		args,
		*exitOnFailure,
		*outputDir,
		*untilDateStr,
		*count,
		*exitCodeMode,
		*maxConcurrent,
	)

	if err != nil {
		fmt.Fprintf(os.Stderr, "shcron: Failed to initiate cli: %v\n", err)
		os.Exit(1)
	}

	if err := appInstance.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "shcron: %v\n", err)
		os.Exit(1)
	}
}
