package sandbox

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
)

// On Linux, srt builds a bubblewrap sandbox with a network namespace of its own and applies its
// seccomp filter. Nothing outside can reach a server inside, so bob-manager hands the helper the
// app's way in as a listening Unix socket on standard input.

// maxSocketPath is the longest path Linux takes for a Unix socket.
const maxSocketPath = 107

// findBwrap finds bubblewrap, which srt builds the sandbox with.
func findBwrap() (string, error) {
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		return "", fmt.Errorf("find bwrap on PATH; install bubblewrap: %w", err)
	}
	return bwrap, nil
}

// findPlatformTools finds socat, which srt bridges its proxies into the sandbox with, and srt's
// seccomp helper for this CPU.
func findPlatformTools(tools *sandboxTools) error {
	var err error
	if tools.socat, err = exec.LookPath("socat"); err != nil {
		return fmt.Errorf("find socat on PATH; install socat: %w", err)
	}
	tools.applySeccomp, err = validateSRTSeccompHelper(tools.srt)
	return err
}

// platformReadable lists the system paths the sandbox may read on Linux: the library folders
// (srt binds the paths it is given after resolving their symlinks, so the loader's names for
// single libraries would be missing), srt's seccomp helper, the certificate authorities and the
// hosts file (srt's bridges are reached as "localhost").
func platformReadable(tools sandboxTools) []string {
	srtPackage := filepath.Dir(filepath.Dir(tools.srt))
	return []string{"/usr/lib64", "/usr/lib", filepath.Join(srtPackage, "vendor", "seccomp"), "/etc/pki", "/etc/ssl", "/etc/hosts"}
}

// applyPlatformSettings names the programs srt runs on Linux.
func applyPlatformSettings(settings *srtSettings, tools sandboxTools) {
	settings.BwrapPath = tools.bwrap
	settings.SocatPath = tools.socat
	settings.Seccomp = &srtSeccomp{ApplyPath: tools.applySeccomp}
}

// thisMachineEndpoint is how OpenCode reaches a port of this machine: as hostLoopbackName
// through srt's proxy, which needs the literal address on the list too.
func thisMachineEndpoint(port string) (string, []string, error) {
	if err := checkHostLoopbackName(); err != nil {
		return "", nil, err
	}
	return hostLoopbackName, []string{"127.0.0.1:" + port}, nil
}

// openWayIn opens the client listener and returns the helper's stdin and OpenCode port.
//
// Clients reach OpenCode through a host TCP listener on bind_address. We forward each
// connection to a Unix socket in the run directory and pass that socket's listening file
// descriptor to the sandbox helper as stdin. The helper accepts connections and forwards
// them to OpenCode on the sandbox's loopback interface.
//
// We must pass only the Unix listener into the sandbox. A TCP socket created on the host
// retains access to the host's network, even inside the sandbox: sandboxed code could
// reconnect it to another host-reachable address, bypassing srt's proxy restrictions.
// The Unix listener cannot be repurposed to make outgoing connections, and srt's filter
// blocks the creation of new Unix sockets.
func openWayIn(runDir, bindAddress string) (*os.File, net.Listener, string, error) {
	path := filepath.Join(runDir, sandboxServerSocket)
	if len(path) > maxSocketPath {
		return nil, nil, "", fmt.Errorf("%s is too long for a Unix socket (%d bytes, at most %d); set XDG_RUNTIME_DIR to a shorter directory",
			path, len(path), maxSocketPath)
	}
	inner, err := net.Listen("unix", path)
	if err != nil {
		return nil, nil, "", err
	}
	inner.(*net.UnixListener).SetUnlinkOnClose(false)
	stdin, err := inner.(*net.UnixListener).File()
	inner.Close()
	if err != nil {
		return nil, nil, "", err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(bindAddress, "0"))
	if err != nil {
		stdin.Close()
		return nil, nil, "", err
	}
	go serveTunnel(listener, dialUnix(path))
	_, port, _ := net.SplitHostPort(sandboxServer)
	return stdin, listener, port, nil
}

// wayInFromStdin is the app's way in the helper serves: the listening Unix socket bob-manager
// hands it as standard input, and never another kind of socket.
func wayInFromStdin() (net.Listener, error) {
	// FileListener keeps a close-on-exec copy, and OpenCode gets no standard input: the helper
	// alone holds the way in.
	listener, err := net.FileListener(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("standard input is not the app's way in: %w", err)
	}
	os.Stdin.Close()
	if _, unix := listener.(*net.UnixListener); !unix {
		listener.Close()
		return nil, errNotUnixWayIn
	}
	return listener, nil
}
