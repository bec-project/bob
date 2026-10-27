package containermanager

import (
	"encoding/binary"
	"testing"
)

// runFilter evaluates a seccomp program the way the kernel does, for one call.
func runFilter(t *testing.T, program []bpfInstruction, arch, nr, arg0 uint32) uint32 {
	t.Helper()
	data := make([]byte, 64)
	binary.LittleEndian.PutUint32(data[seccompNr:], nr)
	binary.LittleEndian.PutUint32(data[seccompDataArch:], arch)
	binary.LittleEndian.PutUint32(data[seccompArg0:], arg0)
	var accumulator uint32
	for pc := 0; pc < len(program); pc++ {
		instruction := program[pc]
		jump := func(taken bool) {
			if taken {
				pc += int(instruction.jt)
			} else {
				pc += int(instruction.jf)
			}
		}
		switch instruction.code {
		case bpfLoadAbs:
			accumulator = binary.LittleEndian.Uint32(data[instruction.k:])
		case bpfJumpEqual:
			jump(accumulator == instruction.k)
		case bpfJumpGE:
			jump(accumulator >= instruction.k)
		case bpfJumpSet:
			jump(accumulator&instruction.k != 0)
		case bpfReturn:
			return instruction.k
		default:
			t.Fatalf("unknown instruction %#x", instruction.code)
		}
	}
	t.Fatal("the program ran off its end")
	return 0
}

func TestSeccompFilterRefusesWhatTheSandboxMustNotDo(t *testing.T) {
	const (
		allow  = seccompAllow
		eperm  = seccompErrno | errnoEPERM
		enosys = seccompErrno | errnoENOSYS
		thread = 0x003d0f00 // CLONE_VM | CLONE_FS | CLONE_FILES | CLONE_SIGHAND | CLONE_THREAD | ...
	)
	for name, arch := range seccompArches {
		t.Run(name, func(t *testing.T) {
			program := arch.program()
			if len(program) > 255 {
				t.Fatal("jumps cannot reach the returns")
			}
			for _, number := range arch.refused {
				if got := runFilter(t, program, arch.audit, number, 0); got != eperm {
					t.Errorf("call %d returned %#x, want EPERM", number, got)
				}
			}
			for _, test := range []struct {
				nr, arg0, want uint32
			}{
				{0, 0, allow},                          // read on x86_64, io_setup on arm64
				{63, 0, allow},                         // uname on x86_64, read on arm64
				{arch.clone, thread, allow},            // a new thread
				{arch.clone, 0x11, allow},              // fork: SIGCHLD
				{arch.clone, 0x10000000 | 0x11, eperm}, // a new user namespace
				{arch.clone, 0x40000000, eperm},        // a new network namespace
				{arch.clone3, 0, enosys},
			} {
				if got := runFilter(t, program, arch.audit, test.nr, test.arg0); got != test.want {
					t.Errorf("call %d (arg %#x) returned %#x, want %#x", test.nr, test.arg0, got, test.want)
				}
			}
			if got := runFilter(t, program, 0x40000003, 1, 0); got != eperm {
				t.Errorf("a call of another ABI returned %#x", got)
			}
			if arch.x32 {
				if got := runFilter(t, program, arch.audit, x32SyscallBit|1, 0); got != eperm {
					t.Errorf("an x32 call returned %#x", got)
				}
			}
		})
	}
}

func TestSeccompFilterBytes(t *testing.T) {
	data, err := seccompFilter()
	if err != nil {
		t.Skip(err)
	}
	if len(data)%bpfInstrSize != 0 || len(data) < 10*bpfInstrSize {
		t.Fatalf("filter of %d bytes", len(data))
	}
	// the last instruction returns ENOSYS
	last := data[len(data)-bpfInstrSize:]
	if binary.LittleEndian.Uint16(last) != bpfReturn || binary.LittleEndian.Uint32(last[4:]) != seccompErrno|errnoENOSYS {
		t.Fatalf("last instruction %x", last)
	}
}
