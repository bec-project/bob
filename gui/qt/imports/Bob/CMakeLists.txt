# The app: its window is the QML module Bob, on the executable itself, so deploying the app
# finds every QML module it imports. qmldir beside it lets Qt Design Studio open the same files.
qt_add_executable(bob-gui MACOSX_BUNDLE ${PROJECT_SOURCE_DIR}/src/main.cpp)
set_source_files_properties(Theme.qml Clock.qml PROPERTIES QT_QML_SINGLETON_TYPE TRUE)

qt_add_qml_module(bob-gui
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
        RestartDialog.qml
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

# QML live preview (Qt 6.12's QmlPreviewMcpServer, for a build with QT_QML_DEBUG) looks for the
# app's preview files in .rcc/qmlpreview beside the app, but Qt writes them where the target is
# made: link them up to the build's top folder, where the app is.
if(NOT CMAKE_CURRENT_BINARY_DIR STREQUAL CMAKE_BINARY_DIR)
    file(MAKE_DIRECTORY ${CMAKE_BINARY_DIR}/.rcc)
    file(CREATE_LINK ${CMAKE_CURRENT_BINARY_DIR}/.rcc/qmlpreview ${CMAKE_BINARY_DIR}/.rcc/qmlpreview SYMBOLIC)
endif()

qt_add_resources(bob-gui "app"
    PREFIX "/"
    BASE ${PROJECT_SOURCE_DIR}
    FILES
        ${PROJECT_SOURCE_DIR}/qtquickcontrols2.conf
)
target_link_libraries(bob-gui PRIVATE Qt6::Gui Qt6::Quick Qt6::QuickControls2 bobbackend bobbackendplugin)
set_target_properties(bob-gui PROPERTIES
    OUTPUT_NAME bob-gui
    # build/bob-gui (build/bob-gui.app on macOS)
    RUNTIME_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}
    MACOSX_BUNDLE_BUNDLE_NAME bob
    MACOSX_BUNDLE_GUI_IDENTIFIER ch.psi.bob
    MACOSX_BUNDLE_BUNDLE_VERSION ${PROJECT_VERSION}
    MACOSX_BUNDLE_SHORT_VERSION_STRING ${PROJECT_VERSION}
    MACOSX_BUNDLE_ICON_FILE bob.icns
)
if(APPLE)
    set(BOB_ICNS ${PROJECT_SOURCE_DIR}/icons/bob.icns)
    target_sources(bob-gui PRIVATE ${BOB_ICNS})
    set_source_files_properties(${BOB_ICNS} PROPERTIES MACOSX_PACKAGE_LOCATION Resources)
endif()

install(TARGETS bob-gui
    BUNDLE DESTINATION .
    RUNTIME DESTINATION ${CMAKE_INSTALL_BINDIR}
)
if(BOB_BUNDLE_QT)
    # qt.conf beside the binary points it at the copied Qt; its RUNPATH is $ORIGIN/../lib
    qt_generate_deploy_qml_app_script(
        TARGET bob-gui
        OUTPUT_SCRIPT bob_deploy_script
        NO_TRANSLATIONS
    )
    install(SCRIPT ${bob_deploy_script})
endif()
if(UNIX AND NOT APPLE)
    install(FILES ${PROJECT_SOURCE_DIR}/packaging/ch.psi.bob.desktop
        DESTINATION ${CMAKE_INSTALL_DATADIR}/applications)
    install(FILES ${PROJECT_SOURCE_DIR}/packaging/ch.psi.bob.metainfo.xml
        DESTINATION ${CMAKE_INSTALL_DATADIR}/metainfo)
    foreach(size 32 128 256 512)
        install(FILES ${PROJECT_SOURCE_DIR}/icons/hicolor/${size}x${size}/ch.psi.bob.png
            DESTINATION ${CMAKE_INSTALL_DATADIR}/icons/hicolor/${size}x${size}/apps)
    endforeach()
endif()
