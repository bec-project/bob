package containermanager

import (
	"runtime"
	"slices"
	"syscall"
	"testing"
)

// The numbers in seccompArches are typed in; this checks them against Go's own table for the
// machine the tests run on.
func TestSeccompNumbersMatchTheKernelTable(t *testing.T) {
	arch, known := seccompArches[runtime.GOARCH]
	if !known {
		t.Skipf("no seccomp filter for %s", runtime.GOARCH)
	}
	if arch.clone != syscall.SYS_CLONE {
		t.Errorf("clone is %d, want %d", arch.clone, syscall.SYS_CLONE)
	}
	for name, number := range map[string]uintptr{
		"unshare": syscall.SYS_UNSHARE,
		"mount":   syscall.SYS_MOUNT, "umount2": syscall.SYS_UMOUNT2, "pivot_root": syscall.SYS_PIVOT_ROOT,
		"add_key": syscall.SYS_ADD_KEY, "request_key": syscall.SYS_REQUEST_KEY, "keyctl": syscall.SYS_KEYCTL,
		"perf_event_open": syscall.SYS_PERF_EVENT_OPEN,
		"init_module":     syscall.SYS_INIT_MODULE, "delete_module": syscall.SYS_DELETE_MODULE,
		"kexec_load": syscall.SYS_KEXEC_LOAD, "reboot": syscall.SYS_REBOOT,
		"swapon": syscall.SYS_SWAPON, "swapoff": syscall.SYS_SWAPOFF, "acct": syscall.SYS_ACCT,
		"quotactl": syscall.SYS_QUOTACTL, "syslog": syscall.SYS_SYSLOG, "vhangup": syscall.SYS_VHANGUP,
		"lookup_dcookie": syscall.SYS_LOOKUP_DCOOKIE,
		"nfsservctl":     syscall.SYS_NFSSERVCTL,
	} {
		if !slices.Contains(arch.refused, uint32(number)) {
			t.Errorf("%s (%d) is not refused", name, number)
		}
	}
}
