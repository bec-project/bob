//go:build !linux

package sandbox

import "errors"

func verifySandboxConfinement() error {
	return errors.New("sandbox confinement can only be verified on Linux")
}

func runConfinementProbe() error {
	return errors.New("sandbox confinement probes run on Linux only")
}
