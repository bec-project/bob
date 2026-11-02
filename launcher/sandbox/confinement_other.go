//go:build !linux && !darwin

package sandbox

import "errors"

func verifySandboxConfinement() error {
	return errors.New("sandbox confinement can only be verified on Linux and macOS")
}

func runConfinementProbe() error {
	return errors.New("sandbox confinement probes run on Linux and macOS only")
}
