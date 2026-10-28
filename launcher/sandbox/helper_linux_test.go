package sandbox

import (
	"syscall"
	"testing"
)

func TestHelperHidesItselfFromTheSandbox(t *testing.T) {
	dumpable := func() uintptr {
		value, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, syscall.PR_GET_DUMPABLE, 0, 0)
		if errno != 0 {
			t.Fatal(errno)
		}
		return value
	}
	defer syscall.RawSyscall(syscall.SYS_PRCTL, syscall.PR_SET_DUMPABLE, dumpable(), 0)
	if err := hideFromSandbox(); err != nil {
		t.Fatal(err)
	}
	if dumpable() != 0 {
		t.Fatal("the helper is still dumpable")
	}
}
