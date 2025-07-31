//go:build windows

package app

import (
	"context"
	"os"
	"os/exec"
)

type OsCommandExecutor struct{}

func (d *OsCommandExecutor) CommandContext(ctx context.Context, name string, arg ...string) CommandRunner {
	cmd := exec.CommandContext(ctx, name, arg...)
	return &DefaultCommandRunner{Cmd: cmd}
}

func (d *OsCommandExecutor) GetEnviron() []string {
	return os.Environ()
}
