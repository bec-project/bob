# Everything about bob and OpenCode, without QML or a GUI: holding bob through its broker,
# OpenCode's REST API and event stream, and the session state they build.
qt_add_library(bobcore STATIC
    bobpaths.cpp bobpaths.h
    bobsession.cpp bobsession.h
    bobsettings.cpp bobsettings.h
    chatclient.cpp chatclient.h
    connection.cpp connection.h
    eventstream.cpp eventstream.h
    opencodeapi.cpp opencodeapi.h
    sessiondata.cpp sessiondata.h
    turns.cpp turns.h
)
target_include_directories(bobcore PUBLIC ${CMAKE_CURRENT_SOURCE_DIR})
target_link_libraries(bobcore PUBLIC Qt6::Core Qt6::Network)
if(BOB_SOURCE_DEFAULTS)
    # gui/qt/src/bob -> the repository root
    get_filename_component(BOB_SOURCE_ROOT "${CMAKE_CURRENT_SOURCE_DIR}/../../../.." ABSOLUTE)
    target_compile_definitions(bobcore PRIVATE BOB_SOURCE_ROOT="${BOB_SOURCE_ROOT}")
endif()
