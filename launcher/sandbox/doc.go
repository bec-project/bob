// Package sandbox manages an isolated OpenCode backend through sandbox-runtime.
//
// Manager starts configured host MCP servers, prepares the enforced OpenCode
// policy, creates the incoming connection tunnel, and launches sandbox-runtime.
// Start returns connection details after authenticated readiness and policy checks.
//
// Calls to one Manager must be serialized. Callers must call Stop to release
// resources after a successful start. Failed starts attempt their own cleanup;
// unresolved host MCP processes remain recorded so Stop can retry.
//
// The manager runs on the host. It invokes the calling executable with
// --sandbox-helper followed by OpenCode's command inside the sandbox. Callers must
// dispatch that mode to RunSandboxHelper, which forwards incoming connections to
// OpenCode and exits with it.
package sandbox
