pragma Singleton
import QtQuick

// Design Studio's stand-in for the app's Plugins (src/gui/plugins.h): one BEC MCP server served by the
// BEC plugin. Its calls fail, so the plugin's panels show how they look without a server.
QtObject {
    id: plugins

    property var servers: [{ name: "bec", url: "http://127.0.0.1:8000/mcp", plugin: "bec" }]
    property var switches: ({})
    property var instanceSettings: ({})

    property QtObject bec: QtObject {
        readonly property string name: "bec"
        readonly property string url: "http://127.0.0.1:8000/mcp"
        property string token: ""
        function call(tool, args, callback, timeoutMs) { callback(undefined, "Design Studio has no MCP server") }
        function tools(callback) { callback([], "Design Studio has no MCP server") }
    }

    signal prefsChanged()

    function server(name) { return name === "bec" ? bec : null }
    function isEnabled(server) { return switches[server] !== false }
    function setEnabled(server, enabled) {
        switches[server] = enabled
        prefsChanged()
    }
    function settings(server) { return instanceSettings[server] || {} }
    function setSettings(server, patch) {
        instanceSettings[server] = Object.assign({}, settings(server), patch)
        prefsChanged()
    }
}
