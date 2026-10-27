//go:build !linux

package containermanager

import "errors"

func dupTo(int, int) error { return errors.New("the bwrap shim runs on Linux only") }
