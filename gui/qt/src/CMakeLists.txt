# The C++ side, in two libraries: bob/ talks to bob (the broker, OpenCode's API and event stream,
# the session state) and knows nothing of QML; gui/ is the QML module Bob.Backend the views read.
add_subdirectory(bob)
add_subdirectory(gui)
