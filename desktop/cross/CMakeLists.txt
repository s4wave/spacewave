cmake_minimum_required(VERSION 3.16)
project(spacewave-helper CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

add_library(spacewave-zero-native-ipc STATIC
    src/zero-native-ipc.cpp
)
target_include_directories(spacewave-zero-native-ipc PUBLIC src)

add_executable(spacewave-zero-native-ipc-test
    src/zero-native-ipc-test.cpp
)
target_link_libraries(spacewave-zero-native-ipc-test PRIVATE spacewave-zero-native-ipc)

if(UNIX)
    target_link_libraries(spacewave-zero-native-ipc-test PRIVATE pthread)
endif()

if(UNIX)
    add_executable(spacewave-update-test
        src/update-manager.cpp
        src/update_manager_test.cc
    )
endif()

enable_testing()
add_test(NAME spacewave-zero-native-ipc-test COMMAND spacewave-zero-native-ipc-test)
if(UNIX)
    add_test(NAME spacewave-update-test COMMAND spacewave-update-test)
endif()

add_executable(spacewave-helper
    src/main.cpp
    src/proto.cpp
    src/pipe-client.cpp
    src/update-manager.cpp
)

if(WIN32)
    target_sources(spacewave-helper PRIVATE
        platform/windows/loading-window-win32.cpp
    )
    target_link_libraries(spacewave-helper PRIVATE comctl32)
elseif(UNIX AND NOT APPLE)
    target_sources(spacewave-helper PRIVATE
        platform/linux/loading-window-x11.cpp
    )
    find_package(X11 REQUIRED)
    target_include_directories(spacewave-helper PRIVATE ${X11_INCLUDE_DIR})
    target_link_libraries(spacewave-helper PRIVATE ${X11_LIBRARIES} pthread)
endif()
