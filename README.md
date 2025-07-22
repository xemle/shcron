# shcron

[![Go Build & Release](https://github.com/xemle/shcron/actions/workflows/release.yml/badge.svg)](https://github.com/xemle/shcron/actions/workflows/release.yml)

**`shcron`** is a flexible command-line tool for periodically executing shell commands. It acts as a lightweight, temporary cron-like scheduler, offering more control than simple shell loops, with support for both time intervals and full cron expressions.

It's designed for scenarios where you need to automate a task for a limited time, without setting up persistent system cron jobs or complex scheduling daemons.

## ✨ Features

* **Flexible Scheduling:**
    * **Interval Patterns:** Simple intervals like `"5s"`, `"1m"`, `"2h"`, `"1d"`.
    * **Cron Expressions:** Full 5-field cron syntax (e.g., `"* * * * *"`, `"0 9 * * 1-5"`).
        * Utilizes a robust cron parser for precise, drift-free scheduling.
* **Controlled Execution:**
    * **Maximum Runs (`-c, --count`):** Limit the total number of times the command executes.
    * **Until Date/Time (`-u, --until`):** Specify a maximum date and time until which the task should run.
    * **Exit on Failure (`-e, --exit-on-failure`):** Stop execution immediately if the command returns a non-zero exit code.
* **Output Management:**
    * **Dump to File (`-o, --output-dir`):** Redirect command output (stdout and stderr) to timestamped log files in a specified directory, one file per run.
* **Custom Exit Codes (`-x, --exit-code`):** Configure `shcron`'s exit code upon termination based on the first/last run's or error's exit code.
* **Graceful Shutdown:** Responds to `Ctrl+C` (`SIGINT`) and `SIGTERM` for clean termination.
* **Cross-Platform:** Built with Go, `shcron` compiles to a single, statically linked binary for Linux, Windows, and macOS.

## 🚀 Installation

### From Source (Recommended for Developers)

To install `shcron` from source, you need to have Go (version 1.22 or higher) installed on your system.

1.  Clone the repository:
    ```bash
    git clone [https://github.com/xemle/shcron.git](https://github.com/xemle/shcron.git)
    cd shcron
    ```
2.  Install the executable to your `GOPATH/bin`:
    ```bash
    go install ./cmd/shcron
    ```
    This will place the `shcron` executable in your `$GOPATH/bin` directory (e.g., `~/go/bin/shcron`). Ensure this directory is in your system's `PATH`.

### From Pre-built Binaries (for Users)

Pre-built binaries for various operating systems and architectures are available in the [GitHub Releases](https://github.com/xemle/shcron/releases) page.

1.  Go to the [latest release](https://github.com/xemle/shcron/releases/latest).
2.  Download the appropriate binary file for your operating system and architecture.
3.  Rename your binary to `shchron`
4.  Move the `shcron` (or `shcron.exe` on Windows) executable to a directory in your system's `PATH` (e.g., `/usr/local/bin` on Linux/macOS, or any directory included in `Path` environment variable on Windows).

## 💡 Usage

```bash
shcron [options] "<pattern>" <command> [args...]
```