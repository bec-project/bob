package containermanager

import "syscall"

// dupTo makes descriptor to a copy of from that survives exec.
func dupTo(from, to int) error {
	return syscall.Dup3(from, to, 0)
}
