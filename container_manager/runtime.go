package containermanager

import (
	"bytes"
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
	return runCommand(ctx, binary, env, false, args...)
}

// Logs need both streams; structured engine responses must contain stdout only.
func commandLogs(ctx context.Context, binary string, args ...string) (string, error) {
	return runCommand(ctx, binary, nil, true, args...)
}

type commandFailure struct {
	err             error
	creationSettled bool
}

func (e *commandFailure) Error() string { return e.err.Error() }
func (e *commandFailure) Unwrap() error { return e.err }

func runCommand(ctx context.Context, binary string, env []string, combine bool, args ...string) (string, error) {
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
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if combine {
		cmd.Stderr = &stdout
	}
	err := cmd.Run()
	if err != nil {
		// No process means no engine request was dispatched. Only recognize
		// specific Docker validation rejections; the daemon error prefix alone
		// can also wrap proxy timeouts while backend creation continues.
		settled := cmd.Process == nil
		var exitErr *exec.ExitError
		if ctx.Err() == nil && filepath.Base(binary) == "docker" && errors.As(err, &exitErr) && exitErr.ExitCode() >= 0 {
			settled = dockerCreateRejected(stderr.String())
		}
		return "", &commandFailure{
			err:             fmt.Errorf("%s failed: %s: %w", filepath.Base(binary), strings.TrimSpace(stdout.String()+stderr.String()), errors.Join(ctx.Err(), err)),
			creationSettled: settled,
		}
	}
	return strings.TrimSpace(stdout.String()), nil
}

func dockerCreateRejected(stderr string) bool {
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimPrefix(line, "docker: ")
		message, ok := strings.CutPrefix(line, "Error response from daemon: ")
		if !ok {
			continue
		}
		// These identify failures before container creation. Unknown messages,
		// including daemon/proxy timeouts and internal errors, stay uncertain.
		for _, rejection := range []string{
			"No such image:", "Conflict. The container name ",
			"invalid mount config for type ", "invalid volume specification:",
			"invalid port specification:", "Invalid container name (",
		} {
			if strings.HasPrefix(message, rejection) {
				return true
			}
		}
	}
	return false
}
