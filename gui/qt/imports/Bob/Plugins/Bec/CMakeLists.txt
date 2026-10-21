# The BEC plugin, as the QML module Bob.Plugins.Bec (README.md beside it). The app loads it by
# name (PluginRegistry), so it imports the app's module Bob without the app importing it.
qt_add_library(bobbec STATIC)
qt_add_qml_module(bobbec
    URI Bob.Plugins.Bec
    VERSION 1.0
    QML_FILES
        BecCard.qml
        BecCodeLines.qml
        BecDevicesPanel.qml
        BecIncidents.qml
        BecIncidentsPanel.qml
        BecLogRecords.qml
        BecPlugin.qml
        BecQueuePanel.qml
        BecSettings.qml
        BecTraceback.qml
    SOURCES
        beccards.cpp beccards.h
        becserver.cpp becserver.h
)
# its server calls go through the app's MCP client (McpServer in src/gui/plugins.h)
target_link_libraries(bobbec PRIVATE bobbackend)
