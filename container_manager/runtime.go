package containermanager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func getExecutable(engine string) (string, error) {
	if engine != "docker" && engine != "podman" {
		return "", errors.New("choose Docker or Podman")
	}
	path, err := exec.LookPath(engine)
	if err != nil {
		return "", fmt.Errorf("find %s on PATH: %w", engine, err)
	}
	return path, nil
}

func command(ctx context.Context, binary string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(os.Environ(), env...)
	// Keep terminal Ctrl-C away from cleanup commands. Cancellation terminates
	// the entire command group, including Docker/Podman runtime helper processes.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return os.ErrProcessDone
			}
			return err
		}
		return nil
	}
	cmd.WaitDelay = 2 * time.Second
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s failed: %s: %w", filepath.Base(binary), strings.TrimSpace(string(output)), errors.Join(ctx.Err(), err))
	}
	return strings.TrimSpace(string(output)), nil
}
