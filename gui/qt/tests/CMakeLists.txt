find_package(Qt6 REQUIRED COMPONENTS Test)

foreach(test tst_bobsession tst_logic)
    qt_add_executable(${test} ${test}.cpp)
    if(EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/${test}.h)
        target_sources(${test} PRIVATE ${test}.h)
    endif()
    target_link_libraries(${test} PRIVATE Qt6::Test bobbackend)
    add_test(NAME ${test} COMMAND ${test})
    # never the user's settings, broker or display
    set_tests_properties(${test} PROPERTIES ENVIRONMENT "QT_QPA_PLATFORM=offscreen")
endforeach()
