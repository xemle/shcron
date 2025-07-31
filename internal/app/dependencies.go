package app

import (
	"context"
	"io"
	"os"
	"os/exec"
	"time"
)

// --- CommandExecutor Interface and Default Implementation ---

// CommandExecutor defines the interface for executing external commands.
// This allows mocking command execution in tests.
type CommandExecutor interface {
	CommandContext(ctx context.Context, name string, arg ...string) CommandRunner
	GetEnviron() []string // Returns a copy of the environment variables
}

// CommandRunner defines the interface for a command that can be run.
// This allows mocking the *exec.Cmd struct.
type CommandRunner interface {
	Run() error
	SetStdout(w io.Writer)
	SetStderr(w io.Writer)
	SetEnv(env []string)
}

// DefaultCommandRunner wraps an *exec.Cmd to implement CommandRunner.
type DefaultCommandRunner struct {
	Cmd *exec.Cmd
}

func (d *DefaultCommandRunner) Run() error {
	return d.Cmd.Run()
}

func (d *DefaultCommandRunner) SetStdout(w io.Writer) {
	d.Cmd.Stdout = w
}

func (d *DefaultCommandRunner) SetStderr(w io.Writer) {
	d.Cmd.Stderr = w
}

func (d *DefaultCommandRunner) SetEnv(env []string) {
	d.Cmd.Env = env
}

// --- FileManager Interface and Default Implementation ---

// FileManager defines the interface for file system operations.
// This allows mocking file system interactions in tests.
type FileManager interface {
	Create(name string) (io.WriteCloser, error)
	Stat(name string) (os.FileInfo, error)
	MkdirAll(path string, perm os.FileMode) error
}

// OsFileManager implements FileManager using os package functions.
type OsFileManager struct{}

// Create creates or truncates the named file.
func (d *OsFileManager) Create(name string) (io.WriteCloser, error) {
	return os.Create(name)
}

// Stat returns a FileInfo describing the named file.
func (d *OsFileManager) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

// MkdirAll creates a directory named path, along with any necessary parents.
func (d *OsFileManager) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// --- Clock Interface and Default Implementation ---

// Clock defines the interface for time-related operations.
// This allows mocking time in tests for predictable scheduling.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// SystemClock implements Clock using time package functions.
type SystemClock struct{}

// Now returns the current local time.
func (d *SystemClock) Now() time.Time {
	return time.Now()
}

// After waits for the duration to elapse and then sends the current time
// on the returned channel.
func (d *SystemClock) After(duration time.Duration) <-chan time.Time {
	return time.After(duration)
}
