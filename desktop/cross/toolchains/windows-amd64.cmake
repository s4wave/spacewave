set(CMAKE_SYSTEM_NAME Windows)
set(CMAKE_SYSTEM_PROCESSOR x86_64)

set(CMAKE_C_COMPILER "/usr/local/bin/zig" "cc")
set(CMAKE_CXX_COMPILER "/usr/local/bin/zig" "c++")
set(CMAKE_C_COMPILER_TARGET x86_64-windows-gnu)
set(CMAKE_CXX_COMPILER_TARGET x86_64-windows-gnu)
set(CMAKE_AR /usr/local/bin/zig-ar)
set(CMAKE_RANLIB /usr/local/bin/zig-ranlib)

set(CMAKE_FIND_ROOT_PATH_MODE_PROGRAM NEVER)
set(CMAKE_FIND_ROOT_PATH_MODE_LIBRARY ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_INCLUDE ONLY)
