pragma Singleton
import QtQuick

// Design Studio's stand-in for the app's Desktop helpers (src/gui/desktop.h).
QtObject {
    property string monoFamily: "monospace"

    function openExternal(url) { return true }
    function copyText(text) {}
    function saveDownload(name, text) { return { path: "/tmp/" + name } }
    function localPath(url) { return String(url).replace("file://", "") }
    function fileName(path) { return String(path).split("/").pop() }
    function formatTokens(value) {
        return value < 1000 ? String(Math.round(value)) : value < 10000 ? (value / 1000).toFixed(1) + "k"
            : value < 1000000 ? Math.round(value / 1000) + "k" : (value / 1000000).toFixed(1) + "M"
    }
    function formatDuration(ms) {
        return ms < 1000 ? Math.round(ms) + " ms" : ms < 60000 ? (ms / 1000).toFixed(1) + " s"
            : Math.floor(ms / 60000) + " min " + Math.round(ms / 1000 % 60) + " s"
    }
    function formatCost(cost) { return "$" + cost.toFixed(2) }
    function relativeTime(ms) {
        const minutes = Math.floor((Date.now() - ms) / 60000)
        return minutes < 1 ? "now" : minutes < 60 ? minutes + "m" : Math.floor(minutes / 60) + "h"
    }
    function clampWidth(width, min, windowWidth) { return Math.round(Math.min(Math.max(width, min), Math.max(min, windowWidth - 480))) }
    function patchHtml(patch, add, del, hunk) {
        return "<pre>" + patch.split("\n").map(line => line.startsWith("+") ? "<span style='color:" + add + "'>" + line + "</span>"
            : line.startsWith("-") ? "<span style='color:" + del + "'>" + line + "</span>" : line).join("\n") + "</pre>"
    }
    function parseConnection(text, password) {
        try { return JSON.parse(text) } catch (error) { return {} }
    }
}
