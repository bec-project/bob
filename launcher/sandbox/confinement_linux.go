package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// verifySandboxConfinement runs before OpenCode. Namespace probes run in a disposable process:
// a missing filter must never change a thread's namespaces in this multithreaded Go helper.
func verifySandboxConfinement() error {
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return fmt.Errorf("read sandbox confinement: %w", err)
	}
	if err := checkSandboxStatus(string(status)); err != nil {
		return err
	}
	helper, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	probe := exec.CommandContext(ctx, helper, "--sandbox-helper", confinementProbeCommand)
	output, err := probe.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sandbox confinement probe failed; refusing to start OpenCode: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if string(output) != confinementProbeSuccess {
		return fmt.Errorf("sandbox confinement probe returned an unexpected result; refusing to start OpenCode")
	}
	return nil
}

// runConfinementProbe must only run as the helper's disposable probe invocation. Every syscall
// uses either bounded resources or invalid pointers/descriptors, so an absent filter cannot
// mount anything or create a running child. A successful unshare ends the probe immediately.
// socket, io_uring, unshare(CLONE_NEWUSER) and setns show that SRT's filter is active: without
// it they fail differently or succeed. mount, umount2, pivot_root and unshare(CLONE_NEWNS) fail
// with EPERM for an unprivileged process anyway; they only confirm that nothing grants them.
func runConfinementProbe() error {
	var setns uintptr
	switch runtime.GOARCH {
	case "amd64":
		setns = 308
	case "arm64":
		setns = 268
	default:
		return fmt.Errorf("sandbox confinement probes are unsupported on %s", runtime.GOARCH)
	}
	for _, family := range []struct {
		name string
		id   uintptr
	}{{"AF_UNIX", syscall.AF_UNIX}, {"AF_VSOCK", 40}} {
		fd, _, errno := syscall.RawSyscall(syscall.SYS_SOCKET, family.id, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
		if errno == 0 {
			syscall.Close(int(fd))
		}
		if err := requireProbeErrno("socket("+family.name+")", errno, syscall.EPERM); err != nil {
			return err
		}
	}
	// The modern syscall numbers are shared by Linux amd64 and arm64. In particular, ENOSYS for
	// io_uring is not accepted: an unavailable syscall does not prove the expected filter is active.
	for _, probe := range []struct {
		name       string
		number     uintptr
		arg0, arg1 uintptr
		want       syscall.Errno
	}{
		{"io_uring_setup", 425, 1, 0, syscall.EPERM},
		{"io_uring_enter", 426, ^uintptr(0), 0, syscall.EPERM},
		{"io_uring_register", 427, ^uintptr(0), 0, syscall.EPERM},
		{"unshare(CLONE_NEWUSER)", syscall.SYS_UNSHARE, syscall.CLONE_NEWUSER, 0, syscall.EPERM},
		{"unshare(CLONE_NEWNS)", syscall.SYS_UNSHARE, syscall.CLONE_NEWNS, 0, syscall.EPERM},
		{"setns", setns, ^uintptr(0), 0, syscall.EPERM},
		{"mount", syscall.SYS_MOUNT, 0, 0, syscall.EPERM},
		{"umount2", syscall.SYS_UMOUNT2, 0, 0, syscall.EPERM},
		{"pivot_root", syscall.SYS_PIVOT_ROOT, 0, 0, syscall.EPERM},
		// SRT deliberately returns ENOSYS for clone3 so runtimes fall back to clone. This is a
		// compatibility check, not independent evidence of confinement on an older kernel.
		{"clone3", 435, 0, 0, syscall.ENOSYS},
	} {
		_, _, errno := syscall.RawSyscall6(probe.number, probe.arg0, probe.arg1, 0, 0, 0, 0)
		if err := requireProbeErrno(probe.name, errno, probe.want); err != nil {
			return err
		}
	}
	return nil
}

func requireProbeErrno(operation string, got, want syscall.Errno) error {
	if got != want {
		return fmt.Errorf("%s returned %s, expected %s", operation, got, want)
	}
	return nil
}
