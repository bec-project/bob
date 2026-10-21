pragma Singleton
import QtQuick

// Design Studio's stand-in for the app's Plugins (src/gui/plugins.h): no MCP server has a plugin.
QtObject {
    id: plugins

    property var servers: []
    property var switches: ({})
    property var instanceSettings: ({})

    signal prefsChanged()

    function server(name) { return null }
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
