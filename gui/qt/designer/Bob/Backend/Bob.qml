pragma Singleton
import QtQuick

// Design Studio's stand-in for the app's Bob (src/gui/bob.h): holding a bob.
QtObject {
    property var status: ({
        state: "joined",
        container: "bob-501-6c788c03",
        config: "/home/me/bob.toml",
        runningConfig: "/home/me/bob.toml",
        connection: { url: "http://127.0.0.1:52997", container: "bob-501-6c788c03" }
    })
    readonly property string state: status.state
    property string config: ""
    property string binary: ""
    property bool autoJoin: true
    property string defaultConfig: "/home/me/bob.toml"
    property string defaultBinary: "/usr/bin/bob-manager"
    property var running: [{
        url: "http://127.0.0.1:52997", password: "", container: "bob-501-6c788c03", runtime: "docker",
        workspace: "/home/me/project", image: "localhost/bob", started: new Date(Date.now() - 120000).toISOString()
    }]
    property bool listing: false
    property string version: "0.0.0"

    function join() {}
    function leave() {}
    function saveSettings(config, binary, autoJoin) { return "" }
    function refreshRunning() {}
}
