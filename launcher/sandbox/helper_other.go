//go:build !linux

package sandbox

// hideFromSandbox does nothing where bob's sandbox does not run.
func hideFromSandbox() error { return nil }
