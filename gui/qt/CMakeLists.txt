cmake_minimum_required(VERSION 3.22)

# The version is stamped by the release build (-DBOB_VERSION=1.2.3).
set(BOB_VERSION "0.0.0" CACHE STRING "Version of the desktop app")
project(bob-gui VERSION ${BOB_VERSION} LANGUAGES CXX)

# Qt 6.12 LTS. RHEL 9 has no Qt that recent (EPEL 9 ships 6.10), so its RPM carries its own copy
# (BOB_BUNDLE_QT).
find_package(Qt6 6.12 REQUIRED COMPONENTS Core Gui Network Qml Quick QuickControls2)
qt_standard_project_setup(REQUIRES 6.12)
qt_policy(SET QTP0004 NEW)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
# Every QML module lands in build/qml/<URI>, where the app and the tools find it.
set(QT_QML_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}/qml)

# A build from a checkout defaults to the checkout's bob.toml and build/bob-manager. Packages turn
# this off: they use bob-manager from the PATH.
option(BOB_SOURCE_DEFAULTS "Default to the bob.toml and bob-manager of this source tree" ON)
# Install Qt's libraries, plugins and QML modules beside the app (bin/, lib/, plugins/, qml/), for a
# package that cannot depend on the system's Qt.
option(BOB_BUNDLE_QT "Install the Qt the app was built with beside it" OFF)

include(GNUInstallDirs)

add_subdirectory(src)
add_subdirectory(imports/Bob)

option(BOB_BUILD_TESTS "Build the unit tests" ON)
if(BOB_BUILD_TESTS)
    enable_testing()
    add_subdirectory(tests)
endif()
