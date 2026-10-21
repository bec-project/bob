pragma Singleton

import QtQuick
import Bob.Backend

// The GUI plugins the app ships, and their instances: bob.toml enables a plugin for an MCP server
// (`plugin = "<id>"`), and each such server gets its own instance, which Settings can switch off.
// Plugins only add views (tool cards, approvals, panels, suggestions); the chat works without them.
//
// A plugin is a QML module with an object that has: pluginId, name, description, suggestions,
// panels [{id, title, icon, width, component}], settings (a Component, or null), and
// describeTool(tool, part, context) and describePermission(tool, request, input), which may return
// nothing; describePermission returns {title, text} or {title, body: {component, properties}}.
// `context` has `server` (an McpServer), `insertPrompt(text)`, `sendPrompt(text)`, and the instance's
// own `settings` with `setSettings(patch)`, which the app keeps.
QtObject {
    id: registry

    // id -> where the plugin's object lives; adding a plugin adds a line here
    readonly property var known: ({})
    // bumped when a switch changes, so the lists below are read again
    property int revision: 0

    property var loaded: ({})
    property var contexts: ({})

    function plugin(id) {
        if (!known[id])
            return null
        if (!loaded[id]) {
            const component = Qt.createComponent(known[id].module, known[id].type)
            if (component.status !== Component.Ready) {
                console.warn("plugin " + id + ": " + component.errorString())
                return null
            }
            loaded[id] = component.createObject(registry)
        }
        return loaded[id]
    }

    // every instance bob.toml asks for, switched on or not: {server, url, plugin}
    readonly property var all: Plugins.servers.filter(server => server.plugin && plugin(server.plugin))
        .map(server => ({ server: server.name, url: server.url, plugin: plugin(server.plugin) }))
    // servers that name a plugin the app does not have: {server, plugin}
    readonly property var missing: Plugins.servers.filter(server => server.plugin && !known[server.plugin])
        .map(server => ({ server: server.name, plugin: server.plugin }))
    readonly property var enabled: {
        revision
        return all.filter(instance => Plugins.isEnabled(instance.server))
    }
    readonly property var suggestions: {
        const list = []
        for (const instance of enabled)
            for (const text of instance.plugin.suggestions || [])
                if (list.indexOf(text) === -1)
                    list.push(text)
        return list
    }
    // the side panels of the instances switched on, named after their server when one plugin
    // serves several
    readonly property var panels: {
        const list = []
        for (const instance of enabled) {
            const shared = enabled.filter(other => other.plugin === instance.plugin).length > 1
            for (const panel of instance.plugin.panels || [])
                list.push({ key: instance.server + "/" + panel.id, title: shared ? panel.title + " · " + instance.server : panel.title,
                            icon: panel.icon, width: panel.width || 340, component: panel.component, instance: instance })
        }
        return list
    }

    property Connections prefs: Connections {
        target: Plugins
        function onPrefsChanged() {
            registry.revision++
            for (const name in registry.contexts)
                registry.contexts[name].settings = Plugins.settings(name)
        }
        function onServersChanged() {
            for (const name in registry.contexts)
                registry.contexts[name].destroy()
            registry.contexts = {}
        }
    }

    // What an instance's views may do with its server and the chat. `settings` is a property, so a
    // binding that reads it follows a change.
    property Component contextType: Component {
        QtObject {
            property string name
            property var server
            property var settings: ({})
            function insertPrompt(text) { Chat.insertPrompt(text) }
            function sendPrompt(text) { Chat.send(text) }
            function setSettings(patch) { Plugins.setSettings(name, patch) }
        }
    }
    function context(instance) {
        const name = instance.server
        if (!contexts[name])
            contexts[name] = contextType.createObject(registry, { name: name, server: Plugins.server(name), settings: Plugins.settings(name) })
        return contexts[name]
    }

    // The instance whose server a tool belongs to, and the tool's own name. OpenCode names MCP tools
    // "<server>_<tool>", so the longest server name that prefixes the id wins.
    function forTool(toolId, instances) {
        let best
        for (const instance of instances || enabled) {
            if (toolId.indexOf(instance.server + "_") === 0 && (!best || instance.server.length > best.server.length))
                best = instance
        }
        return best ? { instance: best, tool: toolId.slice(best.server.length + 1) } : undefined
    }

    // How the instance serving a tool call's server wants it drawn: {label, summary, tone, body:
    // {component, properties}}, or undefined. `item` is a tool row of the thread.
    function toolView(item) {
        const match = forTool(item.tool)
        if (!match || !match.instance.plugin.describeTool)
            return undefined
        const part = { tool: item.tool, state: { status: item.status, input: item.rawInput, output: item.rawOutput } }
        return match.instance.plugin.describeTool(match.tool, part, context(match.instance))
    }

    // How the instance serving the tool behind a permission request wants it drawn: {title, text} or
    // {title, body: {component, properties}}.
    function permissionView(request) {
        const match = request.tool ? forTool(request.tool) : undefined
        if (!match || !match.instance.plugin.describePermission)
            return undefined
        return match.instance.plugin.describePermission(match.tool, request, request.toolInput)
    }
}
