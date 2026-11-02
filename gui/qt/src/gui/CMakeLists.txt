# What the views read, exposed to QML as the module Bob.Backend, on top of bobcore.
qt_add_library(bobbackend STATIC)
qt_add_qml_module(bobbackend
    URI Bob.Backend
    VERSION 1.0
    SOURCES
        bob.cpp bob.h
        chat.cpp chat.h
        chatlogic.cpp chatlogic.h
        desktop.cpp desktop.h
        markdowntext.cpp markdowntext.h
        plugins.cpp plugins.h
        prefs.cpp prefs.h
        sessionlistmodel.cpp sessionlistmodel.h
        threadmodel.cpp threadmodel.h
)
target_include_directories(bobbackend PUBLIC ${CMAKE_CURRENT_SOURCE_DIR})
target_link_libraries(bobbackend PUBLIC bobcore Qt6::Gui Qt6::Qml Qt6::Quick)
target_compile_definitions(bobbackend PUBLIC BOB_VERSION="${PROJECT_VERSION}")
