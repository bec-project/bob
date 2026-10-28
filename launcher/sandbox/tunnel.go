package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// sandbox-runtime gives the sandbox no network and has no way in: nothing outside can reach a
// server inside. bob-manager makes the app's way in outside: a listening Unix socket it hands to
// the sandbox as standard input, which srt passes on to the command it runs (srt's filter refuses
// creating Unix sockets, not accepting on an inherited one), and a TCP listener it forwards to
// that socket. The helper accepts there and pipes each connection to OpenCode on the sandbox's
// own loopback. Never hand the sandbox a TCP or UDP socket made outside: it belongs to the host's
// network, and the sandbox could reconnect it anywhere the host reaches.

// tunnelDialTimeout bounds one connection attempt through a tunnel.
const tunnelDialTimeout = 10 * time.Second

// serveTunnel pipes each connection accepted on listener to one made by dial, until the
// listener is closed.
func serveTunnel(listener net.Listener, dial func(context.Context) (net.Conn, error)) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), tunnelDialTimeout)
			defer cancel()
			peer, err := dial(ctx)
			if err != nil {
				conn.Close()
				return
			}
			pipe(conn, peer)
		}()
	}
}

// pipe copies both ways until both directions end, passing on half-closes, then closes both.
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	copyHalf := func(to, from net.Conn) {
		io.Copy(to, from)
		if closer, ok := to.(interface{ CloseWrite() error }); ok {
			closer.CloseWrite()
		} else {
			to.Close()
		}
		done <- struct{}{}
	}
	go copyHalf(a, b)
	go copyHalf(b, a)
	<-done
	<-done
	a.Close()
	b.Close()
}

func dialTCP(address string) func(context.Context) (net.Conn, error) {
	return func(ctx context.Context) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, "tcp", address)
	}
}

func dialUnix(path string) func(context.Context) (net.Conn, error) {
	return func(ctx context.Context) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, "unix", path)
	}
}

// sandboxServerSocket is the run directory's socket file for the app's way in.
const sandboxServerSocket = "server.sock"

// maxSocketPath is the longest path Linux takes for a Unix socket.
const maxSocketPath = 107

// RunSandboxHelper runs in the sandbox, in place of OpenCode: it serves the app's way in, the
// listening Unix socket it gets as standard input, then runs command, OpenCode, and exits with it.
func RunSandboxHelper(command []string) error {
	if len(command) == 1 && command[0] == confinementProbeCommand {
		if err := runConfinementProbe(); err != nil {
			return err
		}
		_, err := os.Stdout.WriteString(confinementProbeSuccess)
		return err
	}
	if len(command) == 0 {
		return errors.New("no command for the sandbox helper")
	}
	if err := hideFromSandbox(); err != nil {
		return fmt.Errorf("keep the app's way in from the sandbox: %w", err)
	}
	// FileListener keeps a close-on-exec copy, and OpenCode gets no standard input: the helper
	// alone holds the way in.
	listener, err := net.FileListener(os.Stdin)
	if err != nil {
		return fmt.Errorf("standard input is not the app's way in: %w", err)
	}
	os.Stdin.Close()
	if _, unix := listener.(*net.UnixListener); !unix {
		listener.Close()
		return errors.New("the app's way in must be a Unix socket, never one of the host's network")
	}
	defer listener.Close()
	if err := verifySandboxConfinement(); err != nil {
		return err
	}
	go serveTunnel(listener, dialTCP(sandboxServer))

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start OpenCode: %w", err)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		for sig := range signals {
			cmd.Process.Signal(sig)
		}
	}()
	return cmd.Wait()
}
