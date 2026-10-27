package containermanager

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// shimFilterFD is where the shim hands bubblewrap bob's seccomp filter, clear of the descriptors
// sandbox-runtime passes.
const shimFilterFD = 99

// RunBwrapShim runs as sandbox-runtime's bwrap: it stacks bob's seccomp filter onto the sandbox
// srt builds (bubblewrap's --add-seccomp-fd, beside srt's own filters) and then becomes the real
// bwrap with srt's arguments.
func RunBwrapShim(args []string) error {
	real := os.Getenv(shimRealVariable)
	filterPath := os.Getenv(shimSeccompVariable)
	if real == "" || filterPath == "" {
		return errors.New("bob-manager runs as bwrap only for its sandbox")
	}
	filter, err := os.Open(filterPath)
	if err != nil {
		return fmt.Errorf("read bob's seccomp filter: %w", err)
	}
	if err := dupTo(int(filter.Fd()), shimFilterFD); err != nil {
		return err
	}
	argv := append([]string{"bwrap", "--add-seccomp-fd", fmt.Sprint(shimFilterFD)}, args...)
	return syscall.Exec(real, argv, os.Environ())
}
