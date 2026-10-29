//go:build !linux && !darwin

package sandbox

import (
	"errors"
	"net"
	"os"
)

var errUnsupported = errors.New("bob's sandbox runs on Linux and macOS only")

func findBwrap() (string, error)                       { return "", errUnsupported }
func findPlatformTools(*sandboxTools) error            { return errUnsupported }
func platformReadable(sandboxTools) []string           { return nil }
func applyPlatformSettings(*srtSettings, sandboxTools) {}
func thisMachineEndpoint(string) (string, []string, error) {
	return "", nil, errUnsupported
}
func openWayIn(string, string) (*os.File, net.Listener, string, error) {
	return nil, nil, "", errUnsupported
}
func wayInFromStdin() (net.Listener, error) { return nil, errUnsupported }
