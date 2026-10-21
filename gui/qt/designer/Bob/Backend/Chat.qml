pragma Singleton
import QtQuick

// Design Studio's stand-in for the app's Chat (src/chat.h): a connected bob with a few chats and an
// answer that shows every kind of row.
QtObject {
    id: chat

    property string phase: "ready"
    property string connectError: ""
    property var connection: ({ url: "http://127.0.0.1:52997", container: "bob-501-6c788c03", runtime: "docker",
                                workspace: "/home/me/project" })
    property string serverVersion: "1.18.35"
    property string stream: "open"
    property string streamDetail: ""
    property string currentId: "ses_2"
    property string title: "Explain the broker protocol"
    property bool busy: false
    property bool hasMessages: true
    property bool compacting: false
    property string revertID: ""
    property bool running: false
    property var usage: ({ turns: 1, time: "4.2 s", input: "5.5k", output: "380", reasoning: "40", cached: "3.0k",
                           context: "5.4k", contextTokens: 5400, limit: "262k", limitTokens: 262144, fill: 0.02,
                           tools: 1, cost: "" })
    property var modelOptions: [{ key: "psi/qwen", label: "Qwen3.8-27B" }, { key: "psi/gemma", label: "Gemma 4" }]
    property string activeModel: "psi/qwen"
    property string activeModelLabel: "Qwen3.8-27B"
    property string thinkingVariant: "no-thinking"
    property var agentOptions: ["build", "plan"]
    property string activeAgent: "build"
    property var mcpServers: [
        { name: "bec", status: "connected", error: "", tone: "ok", text: "connected", retry: false, detail: "MCP server bec: connected" },
        { name: "docs", status: "failed", error: "connection refused", tone: "error", text: "cannot connect", retry: true,
          detail: "MCP server docs: cannot connect (connection refused). Click to connect again." }
    ]
    property var permissions: []
    property var questions: []
    property var todos: [
        { content: "List the folders", status: "completed" },
        { content: "Read the README", status: "in_progress" },
        { content: "Write the summary", status: "pending" }
    ]
    property var changes: [{ messageID: "msg_1", prompt: "Tidy the launcher", undone: false, files: [
        { file: "launcher/main.go", status: "modified", additions: 2, deletions: 1, patch: "@@ -1,2 +1,3 @@\n package main\n-import \"fmt\"\n+import (\n+\t\"fmt\"" }
    ] }]
    property int changedFiles: 1
    property var notice: ({})
    property string lostReason: ""

    property ListModel sessions: ListModel {
        property string query: ""
        Component.onCompleted: {
            const ago = [60000, 3600000, 3 * 86400000]
            for (let i = 0; i < count; ++i)
                setProperty(i, "updated", Date.now() - ago[i])
        }
        ListElement { sessionId: "ses_1"; title: "Fix the firewall test"; updated: 0; group: "Today"; busy: true; waiting: false }
        ListElement { sessionId: "ses_2"; title: "Explain the broker protocol"; updated: 0; group: "Today"; busy: false; waiting: false }
        ListElement { sessionId: "ses_3"; title: "Read samx"; updated: 0; group: "This week"; busy: false; waiting: true }
    }

    property var thread: [
        { key: "u:1", kind: "user", item: { text: "How does a client hold its lease?", undone: false } },
        { key: "r", kind: "thinking", item: { text: "The broker keeps one lease per connection.", running: false, start: 0, end: 450, expanded: false, undone: false } },
        { key: "t", kind: "tool", item: { label: "Read", tool: "read", summary: "broker.go", status: "completed", running: false, waiting: false,
                                          start: 0, end: 600, input: "{\n  \"filePath\": \"broker.go\"\n}", output: "package main", outputLabel: "Output",
                                          child: "", expanded: false, undone: false } },
        { key: "m#0", kind: "markdown", item: { text: "It keeps the **socket** open: closing it ends the lease.\n\n- one broker per user\n- bob stops 90 s after the last lease", undone: false } },
        { key: "m#1", kind: "code", item: { text: "conn, _ := net.Dial(\"unix\", socket)\nfmt.Fprintln(conn, hello)", language: "go", undone: false } },
        { key: "f:1", kind: "footer", item: { model: "qwen", modelTitle: "psi/qwen", start: 0, end: 4200, live: false, tokens: "5.5k in · 380 out",
                                              tokensTitle: "", reasoning: "40", toolCalls: 1, steps: 2, cost: "", answer: "It keeps the socket open.",
                                              nextID: "", userID: "msg_1", busy: false, undone: false } }
    ]

    signal composerRequest(string text, bool replace)

    function connectTo(connection) {}
    function joinBob() {}
    function leaveBob() {}
    function disconnect() {}
    function selectSession(id) { currentId = id }
    function send(text) {}
    function abort() {}
    function renameSession(id, title) {}
    function deleteSession(id) {}
    function replyPermission(id, reply) {}
    function replyQuestion(id, answers) {}
    function rejectQuestion(id) {}
    function compact() {}
    function revert(messageID) {}
    function unrevert() {}
    function fork(messageID) {}
    function reconnectMcp(name) {}
    function setNotice(text) { notice = { text: text, tone: "info" } }
    function dismissNotice() { notice = {} }
    function setModel(key) { activeModel = key }
    function setAgent(name) { activeAgent = name }
    function setExpanded(key, expanded) {}
    function stepsModel(sessionId) { return [] }
    function copyChat() {}
    function exportChat() {}
    function recall(up, current) { return {} }
    function endRecall() {}
}
