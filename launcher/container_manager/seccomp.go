package containermanager

import (
	"encoding/binary"
	"fmt"
	"runtime"
)

// The sandbox's seccomp filter, which bubblewrap installs (--seccomp) for everything that runs
// in it. It refuses the system calls that reach the kernel's least-used and most attacked parts,
// and that nothing OpenCode or its tools need: new namespaces (and with them, capabilities over
// netfilter and mounts), mounting, the kernel keyring, BPF, perf, io_uring, userfaultfd, modules,
// kexec and the like. Those calls fail with EPERM; clone3 fails with ENOSYS, so the C library
// falls back to clone, whose namespace flags the filter can see.

// seccompArch is one kernel ABI the filter knows: its audit architecture and the numbers of the
// calls it refuses.
type seccompArch struct {
	audit   uint32
	clone   uint32
	clone3  uint32
	refused []uint32
	// x32 marks x86_64, whose x32 ABI sets this bit in the call number.
	x32 bool
}

const (
	auditArchX86_64  = 0xc000003e
	auditArchAarch64 = 0xc00000b7
	x32SyscallBit    = 0x40000000
)

var seccompArches = map[string]seccompArch{
	"amd64": {
		audit: auditArchX86_64, clone: 56, clone3: 435, x32: true,
		refused: []uint32{
			272, 308, // unshare, setns
			165, 166, 155, // mount, umount2, pivot_root
			428, 429, 430, 431, 432, 433, 442, // open_tree, move_mount, fsopen, fsconfig, fsmount, fspick, mount_setattr
			248, 249, 250, // add_key, request_key, keyctl
			321, 298, 323, // bpf, perf_event_open, userfaultfd
			425, 426, 427, // io_uring_setup, io_uring_enter, io_uring_register
			175, 313, 176, // init_module, finit_module, delete_module
			246, 320, 169, // kexec_load, kexec_file_load, reboot
			167, 168, 163, 179, 103, 153, // swapon, swapoff, acct, quotactl, syslog, vhangup
			304, 212, 180, // open_by_handle_at, lookup_dcookie, nfsservctl
			172, 173, 134, // iopl, ioperm, uselib
		},
	},
	"arm64": {
		audit: auditArchAarch64, clone: 220, clone3: 435,
		refused: []uint32{
			97, 268, // unshare, setns
			40, 39, 41, // mount, umount2, pivot_root
			428, 429, 430, 431, 432, 433, 442, // open_tree, move_mount, fsopen, fsconfig, fsmount, fspick, mount_setattr
			217, 218, 219, // add_key, request_key, keyctl
			280, 241, 282, // bpf, perf_event_open, userfaultfd
			425, 426, 427, // io_uring_setup, io_uring_enter, io_uring_register
			105, 273, 106, // init_module, finit_module, delete_module
			104, 294, 142, // kexec_load, kexec_file_load, reboot
			224, 225, 89, 60, 116, 58, // swapon, swapoff, acct, quotactl, syslog, vhangup
			265, 18, 42, // open_by_handle_at, lookup_dcookie, nfsservctl
		},
	},
}

// newNamespaceFlags are clone's CLONE_NEWNS, NEWCGROUP, NEWUTS, NEWIPC, NEWUSER, NEWPID and NEWNET.
const newNamespaceFlags = 0x00020000 | 0x02000000 | 0x04000000 | 0x08000000 | 0x10000000 | 0x20000000 | 0x40000000

// Classic BPF as seccomp reads it (linux/filter.h, linux/seccomp.h).
const (
	bpfLoadAbs   = 0x20 // BPF_LD | BPF_W | BPF_ABS
	bpfJumpEqual = 0x15 // BPF_JMP | BPF_JEQ | BPF_K
	bpfJumpGE    = 0x35 // BPF_JMP | BPF_JGE | BPF_K
	bpfJumpSet   = 0x45 // BPF_JMP | BPF_JSET | BPF_K
	bpfReturn    = 0x06 // BPF_RET | BPF_K

	seccompAllow = 0x7fff0000
	seccompErrno = 0x00050000
	errnoEPERM   = 1
	errnoENOSYS  = 38

	// offsets in struct seccomp_data
	seccompNr       = 0
	seccompDataArch = 4
	seccompArg0     = 16 // the low half on little-endian machines, which both arches are
	bpfInstrSize    = 8
)

type bpfInstruction struct {
	code   uint16
	jt, jf uint8
	k      uint32
}

// seccompFilter is the sandbox's filter for the machine bob-manager was built for, as the bytes
// bubblewrap reads.
func seccompFilter() ([]byte, error) {
	arch, known := seccompArches[runtime.GOARCH]
	if !known {
		return nil, fmt.Errorf("no seccomp filter for %s", runtime.GOARCH)
	}
	program := arch.program()
	data := make([]byte, 0, len(program)*bpfInstrSize)
	for _, instruction := range program {
		data = binary.LittleEndian.AppendUint16(data, instruction.code)
		data = append(data, instruction.jt, instruction.jf)
		data = binary.LittleEndian.AppendUint32(data, instruction.k)
	}
	return data, nil
}

func (arch seccompArch) program() []bpfInstruction {
	// The three returns close the program; jumps count the instructions they skip.
	type target int
	const (
		allow target = iota
		eperm
		enosys
	)
	var program []bpfInstruction
	// jumps to a return are resolved once the program's length is known
	type pending struct {
		at       int
		jt, jf   target
		hasTrue  bool
		hasFalse bool
	}
	var jumps []pending
	add := func(instruction bpfInstruction) { program = append(program, instruction) }
	jumpTrue := func(code uint16, k uint32, to target) {
		jumps = append(jumps, pending{at: len(program), jt: to, hasTrue: true})
		add(bpfInstruction{code: code, k: k})
	}

	// Calls of another ABI (32-bit x86 on x86_64) are refused outright.
	add(bpfInstruction{code: bpfLoadAbs, k: seccompDataArch})
	add(bpfInstruction{code: bpfJumpEqual, jt: 1, k: arch.audit})
	add(bpfInstruction{code: bpfReturn, k: seccompErrno | errnoEPERM})
	add(bpfInstruction{code: bpfLoadAbs, k: seccompNr})
	if arch.x32 {
		jumpTrue(bpfJumpGE, x32SyscallBit, eperm)
	}
	for _, number := range arch.refused {
		jumpTrue(bpfJumpEqual, number, eperm)
	}
	jumpTrue(bpfJumpEqual, arch.clone3, enosys)
	// clone is allowed unless it asks for a new namespace.
	jumps = append(jumps, pending{at: len(program), jf: allow, hasFalse: true})
	add(bpfInstruction{code: bpfJumpEqual, k: arch.clone})
	add(bpfInstruction{code: bpfLoadAbs, k: seccompArg0})
	jumps = append(jumps, pending{at: len(program), jt: eperm, jf: allow, hasTrue: true, hasFalse: true})
	add(bpfInstruction{code: bpfJumpSet, k: newNamespaceFlags})

	returns := len(program)
	add(bpfInstruction{code: bpfReturn, k: seccompAllow})
	add(bpfInstruction{code: bpfReturn, k: seccompErrno | errnoEPERM})
	add(bpfInstruction{code: bpfReturn, k: seccompErrno | errnoENOSYS})
	offset := func(from int, to target) uint8 { return uint8(returns + int(to) - from - 1) }
	for _, jump := range jumps {
		if jump.hasTrue {
			program[jump.at].jt = offset(jump.at, jump.jt)
		}
		if jump.hasFalse {
			program[jump.at].jf = offset(jump.at, jump.jf)
		}
	}
	return program
}
