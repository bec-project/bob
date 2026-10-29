package sandbox

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"syscall"
	"time"
)

// verifySandboxConfinement runs before OpenCode. macOS shows no Seatbelt status a process could
// read, so the helper tries what srt's profile must refuse; each probe touches nothing that
// matters if it were allowed.
func verifySandboxConfinement() error {
	if err := runConfinementProbe(); err != nil {
		return fmt.Errorf("sandbox confinement probe failed; refusing to start OpenCode: %w", err)
	}
	return nil
}

// runConfinementProbe requires srt's macOS profile: no network but srt's proxies and loopback,
// no Unix socket connections, and no writes outside the sandbox's own folders.
func runConfinementProbe() error {
	// 192.0.2.1 is reserved for documentation: nothing answers there, so only Seatbelt can
	// refuse the connection at once.
	conn, err := net.DialTimeout("tcp", "192.0.2.1:9", 2*time.Second)
	if err == nil {
		conn.Close()
	}
	if !errors.Is(err, syscall.EPERM) {
		return fmt.Errorf("a connection to 192.0.2.1 returned %v, expected EPERM", err)
	}
	unix, err := net.Dial("unixgram", "/var/run/syslog")
	if err == nil {
		unix.Close()
	}
	if !errors.Is(err, syscall.EPERM) {
		return fmt.Errorf("a connection to /var/run/syslog returned %v, expected EPERM", err)
	}
	path := "/private/var/tmp/bob-confinement-probe-" + strconv.Itoa(os.Getpid())
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		file.Close()
		os.Remove(path)
	}
	if !errors.Is(err, syscall.EPERM) {
		return fmt.Errorf("creating %s returned %v, expected EPERM", path, err)
	}
	return nil
}
