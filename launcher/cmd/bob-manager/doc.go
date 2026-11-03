// Command bob-manager locates and runs a per-user broker for a shared OpenCode backend.
//
// In its default mode, it locates or starts a detached broker, prints the broker's
// Unix socket path, and exits. With --broker, it serves client leases and manages
// one backend through sandbox.Manager. With --sandbox-helper, it runs inside the
// sandbox, forwards incoming connections, and launches OpenCode.
//
// The broker socket carries lifecycle requests and replies. OpenCode API traffic
// uses a separate HTTP endpoint returned to clients by the broker.
package main
