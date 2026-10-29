package sandbox

import "syscall"

// hideFromSandbox makes the helper non-dumpable. The other processes of the sandbox run as the
// same user, but can then neither trace the helper nor take its descriptors (/proc/<pid>/fd,
// pidfd_getfd), so the app's way in stays the helper's alone. OpenCode, which the helper starts,
// is dumpable again after its exec.
func hideFromSandbox() error {
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, syscall.PR_SET_DUMPABLE, 0, 0); errno != 0 {
		return errno
	}
	return nil
}
