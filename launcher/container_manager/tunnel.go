package containermanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// A bubblewrap sandbox has no network: OpenCode reaches the provider and the MCP servers, and
// clients reach OpenCode, only through Unix sockets in a directory both sides see. A tunnel is a
// listener whose every connection is piped to a fresh connection made by dial.

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

// dialEndpoint dials an endpoint's target, refusing an address it must not reach. The check runs
// on the address actually dialled, after the name was resolved, so a name that later resolves
// elsewhere is caught too.
func dialEndpoint(endpoint sandboxEndpoint) func(context.Context) (net.Conn, error) {
	return func(ctx context.Context) (net.Conn, error) {
		dialer := net.Dialer{Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			return checkEndpointAddress(net.ParseIP(host), endpoint.thisMachine)
		}}
		return dialer.DialContext(ctx, "tcp", endpoint.target)
	}
}

// awsMetadataIPv6 is the cloud metadata service's IPv6 address; its IPv4 one is link-local.
var awsMetadataIPv6 = net.ParseIP("fd00:ec2::254")

// checkEndpointAddress refuses addresses an endpoint must not lead to: this machine's loopback
// (except for host.containers.internal, which means this machine), link-local addresses, which
// include the cloud metadata service at 169.254.169.254, and unspecified, multicast and broadcast
// addresses.
func checkEndpointAddress(ip net.IP, thisMachine bool) error {
	if ip == nil {
		return errors.New("not an IP address")
	}
	if mapped := ip.To4(); mapped != nil {
		ip = mapped
	}
	switch {
	case ip.IsLoopback() && thisMachine:
		return nil
	case ip.IsLoopback():
		return fmt.Errorf("%s is this machine's loopback; reach the host as host.containers.internal", ip)
	case ip.IsLinkLocalUnicast(), ip.Equal(awsMetadataIPv6):
		return fmt.Errorf("%s is link-local, where cloud metadata services live", ip)
	case ip.IsUnspecified(), ip.IsMulticast(), ip.IsLinkLocalMulticast(), ip.Equal(net.IPv4bcast), ip.To4() != nil && ip.To4()[0] == 0:
		return fmt.Errorf("%s is not a host address", ip)
	}
	return nil
}

func dialUnix(path string) func(context.Context) (net.Conn, error) {
	return func(ctx context.Context) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, "unix", path)
	}
}

// sandboxPlanVariable carries the sandboxPlan from bob-manager to its helper in the sandbox.
const sandboxPlanVariable = "BOB_SANDBOX_PLAN"

// sandboxRunDir is where the sandbox sees the sockets folder of bob-manager's run directory.
const sandboxRunDir = "/run/bob"

// sandboxServerSocket is the socket file the helper connects to OpenCode's port.
const sandboxServerSocket = "server.sock"

// sandboxPlan is what the helper in the sandbox sets up before it starts OpenCode.
type sandboxPlan struct {
	// Endpoints are the provider and MCP servers OpenCode may reach.
	Endpoints []sandboxEndpoint `json:"endpoints"`
	// Server is OpenCode's address in the sandbox, which clients reach through
	// sandboxServerSocket.
	Server string `json:"server"`
	// Command starts OpenCode.
	Command []string `json:"command"`
}

// sandboxEndpoint is one endpoint OpenCode may reach. In the sandbox, the helper accepts
// OpenCode's connections on Listen; outside, bob-manager dials target for each of them.
type sandboxEndpoint struct {
	Listen string `json:"listen"`
	// Socket is the tunnel's socket file in the run directory.
	Socket string `json:"socket"`
	target string
	// thisMachine marks host.containers.internal, which bob-manager reaches on loopback.
	thisMachine bool
}

// RunSandboxHelper runs in the sandbox, in place of OpenCode: it opens the tunnels the plan in
// its environment names, then runs OpenCode and exits with it.
func RunSandboxHelper() error {
	var plan sandboxPlan
	if err := json.Unmarshal([]byte(os.Getenv(sandboxPlanVariable)), &plan); err != nil {
		return fmt.Errorf("read the sandbox plan: %w", err)
	}
	if len(plan.Command) == 0 {
		return errors.New("the sandbox plan has no command")
	}
	for _, endpoint := range plan.Endpoints {
		listener, err := net.Listen("tcp", endpoint.Listen)
		if err != nil {
			return fmt.Errorf("listen for %s: %w", endpoint.Socket, err)
		}
		go serveTunnel(listener, dialUnix(filepath.Join(sandboxRunDir, endpoint.Socket)))
	}
	server, err := net.Listen("unix", filepath.Join(sandboxRunDir, sandboxServerSocket))
	if err != nil {
		return fmt.Errorf("listen for clients: %w", err)
	}
	go serveTunnel(server, dialTCP(plan.Server))

	cmd := exec.Command(plan.Command[0], plan.Command[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, os.Stdout, os.Stderr
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, sandboxPlanVariable+"=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
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
