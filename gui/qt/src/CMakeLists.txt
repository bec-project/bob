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
