pragma Singleton
import QtQuick

// Design Studio's stand-in for the plugin's C++ BecServer (becserver.h): every call fails, so the
// panels show how they look without a server.
QtObject {
    function callResult(server, tool, args, done) { done(undefined, "Design Studio has no MCP server") }
    function tools(server, done) { done({}, "Design Studio has no MCP server") }
}
