# The window, as the QML module Bob. qmldir beside it lets Qt Design Studio open the same files.
set_source_files_properties(Theme.qml Clock.qml PROPERTIES QT_QML_SINGLETON_TYPE TRUE)

qt_add_library(bobui STATIC)
qt_add_qml_module(bobui
    URI Bob
    VERSION 1.0
    QML_FILES
        AppButton.qml
        AppComboBox.qml
        AppTextField.qml
        Banner.qml
        ChangesPanel.qml
        ChatView.qml
        Chip.qml
        Clock.qml
        CodeBlock.qml
        Composer.qml
        ConfirmDialog.qml
        ConnectScreen.qml
        CopyButton.qml
        Disclosure.qml
        Dot.qml
        EmptyChat.qml
        Header.qml
        Icon.qml
        IconButton.qml
        Kbd.qml
        Main.qml
        MarkdownView.qml
        McpChip.qml
        PermissionCard.qml
        QuestionCard.qml
        RequestCard.qml
        Requests.qml
        SessionRow.qml
        SettingsDialog.qml
        Sidebar.qml
        Spinner.qml
        Splitter.qml
        SubAgentRun.qml
        Theme.qml
        ThinkingBlock.qml
        ThreadItem.qml
        ThreadView.qml
        Todos.qml
        ToggleSwitch.qml
        ToolCall.qml
        TurnFooter.qml
        UsageBar.qml
        Working.qml
    RESOURCES
        images/bob.png
    IMPORTS
        Bob.Backend
)
target_link_libraries(bobui PRIVATE Qt6::Quick Qt6::QuickControls2)
