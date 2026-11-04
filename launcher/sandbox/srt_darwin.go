package sandbox

import (
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// On macOS, srt confines the sandbox with Seatbelt (sandbox-exec). The sandbox shares the host's
// network: srt's profile lets it reach only srt's proxy ports, and with allowLocalBinding it may
// also bind loopback ports and connect to any of them. OpenCode cannot serve on a Unix socket, so
// on macOS it serves on a loopback port of this machine, and bob-manager forwards the app's way
// in to it. That also lets the sandbox reach every other loopback port, such as a local Redis:
// macOS is a development platform for bob, weaker than Linux.

// maxSocketPath is the longest path macOS takes for a Unix socket.
const maxSocketPath = 103

// findBwrap finds nothing: srt needs no bubblewrap on macOS.
func findBwrap() (string, error) { return "", nil }

// findPlatformTools finds nothing more: on macOS srt needs neither socat nor a seccomp helper.
func findPlatformTools(*sandboxTools) error { return nil }

// platformReadable lists the system paths the sandbox may read on macOS: the system libraries,
// frameworks and dyld's shared cache, ICU's data (OpenCode's JavaScript engine stops at start
// without it), the certificate authorities, the hosts file and the time zones.
func platformReadable(sandboxTools) []string {
	return []string{"/usr/lib", "/usr/share/icu", "/System", "/private/etc/ssl", "/private/etc/hosts", "/private/var/db/timezone"}
}

// applyPlatformSettings lets OpenCode serve on a loopback port, the only way a server in srt's
// macOS sandbox can be reached.
func applyPlatformSettings(settings *srtSettings, _ sandboxTools) {
	settings.Network.AllowLocalBinding = true
}

// thisMachineEndpoint is how OpenCode reaches a port of this machine: directly, on loopback,
// which allowLocalBinding opens; srt's NO_PROXY keeps such connections away from its proxy.
func thisMachineEndpoint(string) (string, []string, error) {
	return "127.0.0.1", nil, nil
}

// sandboxCommandEnvironment uses numeric loopback for srt's proxies because
// localhost lookup can fail inside Seatbelt. Preserve srt's ports, credentials
// and NO_PROXY rules; only the spelling of its loopback address changes.
func sandboxCommandEnvironment(environment []string) []string {
	environment = append([]string(nil), environment...)
	for index, entry := range environment {
		name, value, _ := strings.Cut(entry, "=")
		switch strings.ToLower(name) {
		case "http_proxy", "https_proxy", "all_proxy", "ftp_proxy":
			endpoint, err := url.Parse(value)
			if err != nil || endpoint.Hostname() != "localhost" {
				continue
			}
			port := endpoint.Port()
			endpoint.Host = "127.0.0.1"
			if port != "" {
				endpoint.Host = net.JoinHostPort("127.0.0.1", port)
			}
			environment[index] = name + "=" + endpoint.String()
		}
	}
	return environment
}

// openWayIn makes the app's way in: a TCP listener on bind_address, forwarded to the loopback
// port OpenCode serves on. It returns no standard input for the helper, the listener and that
// port.
func openWayIn(_, bindAddress string) (*os.File, net.Listener, string, error) {
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, "", err
	}
	port := strconv.Itoa(free.Addr().(*net.TCPAddr).Port)
	free.Close()
	listener, err := net.Listen("tcp", net.JoinHostPort(bindAddress, "0"))
	if err != nil {
		return nil, nil, "", err
	}
	go serveTunnel(listener, dialTCP(net.JoinHostPort("127.0.0.1", port)))
	return nil, listener, port, nil
}

// wayInFromStdin serves nothing on macOS: OpenCode serves the way in on loopback itself.
func wayInFromStdin() (net.Listener, error) { return nil, nil }
