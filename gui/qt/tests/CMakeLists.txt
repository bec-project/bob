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

# the BEC plugin's cards, against the recorded tool results in the plugin's folder
set(bec_dir ${PROJECT_SOURCE_DIR}/imports/Bob/Plugins/Bec)
qt_add_executable(tst_bec tst_bec.cpp ${bec_dir}/beccards.cpp ${bec_dir}/beccards.h)
target_include_directories(tst_bec PRIVATE ${bec_dir})
target_compile_definitions(tst_bec PRIVATE BEC_FIXTURES="${bec_dir}/fixtures")
target_link_libraries(tst_bec PRIVATE Qt6::Test Qt6::Qml)
add_test(NAME tst_bec COMMAND tst_bec)
set_tests_properties(tst_bec PROPERTIES TIMEOUT 120)
