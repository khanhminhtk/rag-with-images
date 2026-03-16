cmake_minimum_required(VERSION 3.10)
project(EdgeSentinel VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

set(DOMAIN_HEADERS
        domain/value_objects/ConfigLoader.hpp
)

set(UTILS_SOURCES
        utils/configloader.cpp
        utils/configloader.hpp
)

set(APPLICATION_SOURCES
        # Thêm source file (.cpp) tại đây
)

add_library(edge_sentinel_lib STATIC
        ${APPLICATION_SOURCES}
        ${DOMAIN_HEADERS}
        ${UTILS_SOURCES}
)

target_include_directories(edge_sentinel_lib
        PUBLIC
        $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}>
        $<INSTALL_INTERFACE:include>
)

target_link_libraries(edge_sentinel_lib
        PUBLIC
        onnxruntime
        yaml-cpp
        PRIVATE
)

if(NOT MSVC)
    target_compile_options(edge_sentinel_lib PRIVATE -O3 -march=native -Wall -Wextra)
endif()

if(COMMAND set_project_warnings)
    set_project_warnings(edge_sentinel_lib)
endif()

if(COMMAND enable_sanitizers)
    enable_sanitizers(edge_sentinel_lib)
endif()

add_executable(edge_sentinel main.cpp)

target_link_libraries(edge_sentinel
        PRIVATE
        edge_sentinel_lib
)
