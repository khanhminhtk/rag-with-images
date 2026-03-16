include(FetchContent)

message(STATUS "yaml-cpp not found locally, fetching from source...")

FetchContent_Declare(
        yaml-cpp
        GIT_REPOSITORY https://github.com/jbeder/yaml-cpp.git
        GIT_TAG 0.8.0
)

set(YAML_CPP_BUILD_TESTS OFF CACHE BOOL "" FORCE)
set(YAML_CPP_BUILD_TOOLS OFF CACHE BOOL "" FORCE)
set(YAML_CPP_BUILD_CONTRIB OFF CACHE BOOL "" FORCE)
set(CMAKE_POLICY_DEFAULT_CMP0077 NEW)

if(DEFINED BUILD_TESTING)
    set(_EDGE_SENTINEL_PREV_BUILD_TESTING ${BUILD_TESTING})
    set(_EDGE_SENTINEL_HAD_BUILD_TESTING TRUE)
endif()

set(BUILD_TESTING OFF CACHE BOOL "" FORCE)

FetchContent_MakeAvailable(yaml-cpp)

if(TARGET yaml-cpp AND CMAKE_CXX_COMPILER_ID MATCHES "GNU|Clang")
    target_compile_options(yaml-cpp PRIVATE -include cstdint)
endif()

if(_EDGE_SENTINEL_HAD_BUILD_TESTING)
    set(BUILD_TESTING ${_EDGE_SENTINEL_PREV_BUILD_TESTING} CACHE BOOL "" FORCE)
else()
    unset(BUILD_TESTING CACHE)
endif()


if(NOT TARGET onnxruntime)
    if(CMAKE_SYSTEM_PROCESSOR MATCHES "aarch64|arm64")
        set(ORT_ARCH "linux-aarch64")
    else()
        set(ORT_ARCH "linux-x64")
    endif()

    set(ORT_VER "1.18.0")
    set(ORT_FOLDER_NAME "onnxruntime-${ORT_ARCH}-${ORT_VER}")
    set(ORT_URL "https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VER}/${ORT_FOLDER_NAME}.tgz")

    message(STATUS "Downloading OnnxRuntime (${ORT_ARCH})...")

    FetchContent_Declare(
            onnxruntime_prebuilt
            URL ${ORT_URL}
            DOWNLOAD_EXTRACT_TIMESTAMP TRUE
    )
    FetchContent_MakeAvailable(onnxruntime_prebuilt)

    set(ORT_ROOT "${onnxruntime_prebuilt_SOURCE_DIR}")

    if(EXISTS "${ORT_ROOT}/${ORT_FOLDER_NAME}/include/onnxruntime_cxx_api.h")
        set(ORT_INCLUDE_DIR "${ORT_ROOT}/${ORT_FOLDER_NAME}/include")
        set(ORT_LIB_DIR     "${ORT_ROOT}/${ORT_FOLDER_NAME}/lib")
        message(STATUS "  -> Detected nested folder structure. Fixing path...")
    else()
        set(ORT_INCLUDE_DIR "${ORT_ROOT}/include")
        set(ORT_LIB_DIR     "${ORT_ROOT}/lib")
    endif()

    file(GLOB ORT_LIB_FILE "${ORT_LIB_DIR}/libonnxruntime.so*")
    list(GET ORT_LIB_FILE 0 ORT_FINAL_LIB)

    if(NOT EXISTS "${ORT_FINAL_LIB}")
        message(FATAL_ERROR "Could not find libonnxruntime.so in ${ORT_LIB_DIR}")
    endif()

    add_library(onnxruntime SHARED IMPORTED GLOBAL)
    set_target_properties(onnxruntime PROPERTIES
            IMPORTED_LOCATION "${ORT_FINAL_LIB}"
            INTERFACE_INCLUDE_DIRECTORIES "${ORT_INCLUDE_DIR}"
    )

    message(STATUS "OnnxRuntime Setup OK:")
    message(STATUS "  - Lib: ${ORT_FINAL_LIB}")
    message(STATUS "  - Include: ${ORT_INCLUDE_DIR}")
endif()
