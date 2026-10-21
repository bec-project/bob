find_package(Qt6 REQUIRED COMPONENTS Test)

# A test links only the library it tests: bobcore for bob's side, bobbackend for the GUI's.
function(bob_test test library)
    qt_add_executable(${test} ${test}.cpp)
    if(EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/${test}.h)
        target_sources(${test} PRIVATE ${test}.h)
    endif()
    target_link_libraries(${test} PRIVATE Qt6::Test ${library})
    add_test(NAME ${test} COMMAND ${test})
    # never the user's settings, broker or display; a test that hangs fails instead of holding CI
    set_tests_properties(${test} PROPERTIES ENVIRONMENT "QT_QPA_PLATFORM=offscreen" TIMEOUT 120)
endfunction()

bob_test(tst_bobsession bobcore)
bob_test(tst_chatclient bobcore)
bob_test(tst_mcp bobcore)
bob_test(tst_logic bobbackend)
