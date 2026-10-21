# The C++ side: everything about bob and OpenCode, exposed to QML as the module Bob.Backend.
qt_add_library(bobbackend STATIC)
qt_add_qml_module(bobbackend
    URI Bob.Backend
    VERSION 1.0
    SOURCES
        bob.cpp bob.h
        bobpaths.cpp bobpaths.h
        bobsession.cpp bobsession.h
        bobsettings.cpp bobsettings.h
        chat.cpp chat.h
        chatlogic.cpp chatlogic.h
        desktop.cpp desktop.h
        eventstream.cpp eventstream.h
        markdowntext.cpp markdowntext.h
        opencodeapi.cpp opencodeapi.h
        prefs.cpp prefs.h
        runningbobs.cpp runningbobs.h
        sessiondata.cpp sessiondata.h
        sessionlistmodel.cpp sessionlistmodel.h
        threadmodel.cpp threadmodel.h
        turns.cpp turns.h
)
target_include_directories(bobbackend PUBLIC ${CMAKE_CURRENT_SOURCE_DIR})
target_link_libraries(bobbackend PUBLIC Qt6::Core Qt6::Gui Qt6::Network Qt6::Qml Qt6::Quick)
target_compile_definitions(bobbackend PUBLIC BOB_VERSION="${PROJECT_VERSION}")
if(BOB_SOURCE_DEFAULTS)
    # gui/qt/src -> the repository root
    get_filename_component(BOB_SOURCE_ROOT "${CMAKE_CURRENT_SOURCE_DIR}/../../.." ABSOLUTE)
    target_compile_definitions(bobbackend PRIVATE BOB_SOURCE_ROOT="${BOB_SOURCE_ROOT}")
endif()

qt_add_executable(bob-gui MACOSX_BUNDLE main.cpp)
qt_add_resources(bob-gui "app"
    PREFIX "/"
    BASE ${CMAKE_CURRENT_SOURCE_DIR}/..
    FILES
        ../qtquickcontrols2.conf
)
target_link_libraries(bob-gui PRIVATE
    Qt6::Gui Qt6::Quick Qt6::QuickControls2 bobbackend bobbackendplugin bobuiplugin)
set_target_properties(bob-gui PROPERTIES
    OUTPUT_NAME bob-gui
    MACOSX_BUNDLE_BUNDLE_NAME bob
    MACOSX_BUNDLE_GUI_IDENTIFIER ch.psi.bob
    MACOSX_BUNDLE_BUNDLE_VERSION ${PROJECT_VERSION}
    MACOSX_BUNDLE_SHORT_VERSION_STRING ${PROJECT_VERSION}
    MACOSX_BUNDLE_ICON_FILE bob.icns
)
if(APPLE)
    set(BOB_ICNS ${CMAKE_CURRENT_SOURCE_DIR}/../icons/bob.icns)
    target_sources(bob-gui PRIVATE ${BOB_ICNS})
    set_source_files_properties(${BOB_ICNS} PROPERTIES MACOSX_PACKAGE_LOCATION Resources)
endif()

install(TARGETS bob-gui
    BUNDLE DESTINATION .
    RUNTIME DESTINATION ${CMAKE_INSTALL_BINDIR}
)
if(UNIX AND NOT APPLE)
    install(FILES ../packaging/ch.psi.bob.desktop DESTINATION ${CMAKE_INSTALL_DATADIR}/applications)
    install(FILES ../packaging/ch.psi.bob.metainfo.xml DESTINATION ${CMAKE_INSTALL_DATADIR}/metainfo)
    foreach(size 32 128 256 512)
        install(FILES ../icons/hicolor/${size}x${size}/ch.psi.bob.png
            DESTINATION ${CMAKE_INSTALL_DATADIR}/icons/hicolor/${size}x${size}/apps)
    endforeach()
endif()
