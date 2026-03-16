cmake_minimum_required(VERSION 3.17)

project(
        onnx,
        VERSION 1.0.0
        DESCRIPTION "Inference model DL with onnx cpp"
        LANGUAGES CXX
)

set(CMAKE_POLICY_VERSION_MINIMUM 3.5)
list(APPEND CMAKE_MODULE_PATH "${CMAKE_CURRENT_SOURCE_DIR}/cmake")

include(CompilerWarnings)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

include(Dependencies)

add_subdirectory(src)