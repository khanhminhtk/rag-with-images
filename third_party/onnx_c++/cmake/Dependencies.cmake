include(FetchContent)

option(ORT_USE_CUDA "Download and link ONNX Runtime GPU package" OFF)
set(ORT_VER "1.18.0" CACHE STRING "ONNX Runtime version")
set(ORT_URL_OVERRIDE "" CACHE STRING "Custom ONNX Runtime package URL")
set(ORT_ARCHIVE_PATH "" CACHE FILEPATH "Local ONNX Runtime archive (.tgz)")

# ── yaml-cpp ────────────────────────────────────
message(STATUS "Fetching yaml-cpp...")

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
    set(_PREV_BUILD_TESTING ${BUILD_TESTING})
    set(_HAD_BUILD_TESTING TRUE)
endif()
set(BUILD_TESTING OFF CACHE BOOL "" FORCE)

FetchContent_MakeAvailable(yaml-cpp)

if(TARGET yaml-cpp AND CMAKE_CXX_COMPILER_ID MATCHES "GNU|Clang")
    target_compile_options(yaml-cpp PRIVATE -include cstdint)
endif()

if(_HAD_BUILD_TESTING)
    set(BUILD_TESTING ${_PREV_BUILD_TESTING} CACHE BOOL "" FORCE)
else()
    unset(BUILD_TESTING CACHE)
endif()

# ── ONNX Runtime (pre-built) ───────────────────
if(NOT TARGET onnxruntime)
    if(CMAKE_SYSTEM_PROCESSOR MATCHES "aarch64|arm64")
        set(ORT_ARCH "linux-aarch64")
    else()
        set(ORT_ARCH "linux-x64")
    endif()

    if(ORT_USE_CUDA)
        if(NOT ORT_ARCH STREQUAL "linux-x64")
            message(FATAL_ERROR "ORT_USE_CUDA=ON is only supported for linux-x64 prebuilt package in this project")
        endif()
        set(ORT_FOLDER "onnxruntime-${ORT_ARCH}-gpu-${ORT_VER}")
        set(ORT_VARIANT "gpu")
    else()
        set(ORT_FOLDER "onnxruntime-${ORT_ARCH}-${ORT_VER}")
        set(ORT_VARIANT "cpu")
    endif()

    if(ORT_ARCHIVE_PATH)
        if(NOT EXISTS "${ORT_ARCHIVE_PATH}")
            message(FATAL_ERROR "ORT_ARCHIVE_PATH does not exist: ${ORT_ARCHIVE_PATH}")
        endif()
        set(ORT_URL "file://${ORT_ARCHIVE_PATH}")
    elseif(ORT_URL_OVERRIDE)
        set(ORT_URL "${ORT_URL_OVERRIDE}")
    else()
        set(ORT_URL "https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VER}/${ORT_FOLDER}.tgz")
    endif()

    message(STATUS "Downloading OnnxRuntime ${ORT_VER} (${ORT_ARCH}, variant=${ORT_VARIANT})")

    FetchContent_Declare(
        onnxruntime_prebuilt
        URL ${ORT_URL}
        DOWNLOAD_EXTRACT_TIMESTAMP TRUE
    )
    FetchContent_MakeAvailable(onnxruntime_prebuilt)

    set(ORT_ROOT "${onnxruntime_prebuilt_SOURCE_DIR}")

    if(EXISTS "${ORT_ROOT}/${ORT_FOLDER}/include/onnxruntime_cxx_api.h")
        set(ORT_INCLUDE "${ORT_ROOT}/${ORT_FOLDER}/include")
        set(ORT_LIBDIR  "${ORT_ROOT}/${ORT_FOLDER}/lib")
    else()
        set(ORT_INCLUDE "${ORT_ROOT}/include")
        set(ORT_LIBDIR  "${ORT_ROOT}/lib")
    endif()

    file(GLOB ORT_LIB_FILE "${ORT_LIBDIR}/libonnxruntime.so*")
    list(GET ORT_LIB_FILE 0 ORT_FINAL_LIB)

    if(NOT EXISTS "${ORT_FINAL_LIB}")
        message(FATAL_ERROR "Could not find libonnxruntime.so in ${ORT_LIBDIR}")
    endif()

    if(ORT_USE_CUDA)
        if(NOT EXISTS "${ORT_LIBDIR}/libonnxruntime_providers_cuda.so")
            message(FATAL_ERROR
                "ORT_USE_CUDA=ON but CUDA provider library not found in ${ORT_LIBDIR}. "
                "The archive must be ONNX Runtime GPU package.")
        endif()
    endif()

    add_library(onnxruntime SHARED IMPORTED GLOBAL)
    set_target_properties(onnxruntime PROPERTIES
        IMPORTED_LOCATION "${ORT_FINAL_LIB}"
        INTERFACE_INCLUDE_DIRECTORIES "${ORT_INCLUDE}"
    )

    message(STATUS "OnnxRuntime: ${ORT_FINAL_LIB}")
    message(STATUS "OnnxRuntime package folder: ${ORT_FOLDER}")
endif()

# ── spdlog ──────────────────────────────────────
FetchContent_Declare(
    spdlog
    GIT_REPOSITORY https://github.com/gabime/spdlog.git
    GIT_TAG v1.12.0
)
FetchContent_MakeAvailable(spdlog)

# ── OpenCV ──────────────────────────────────────
find_package(OpenCV REQUIRED COMPONENTS core imgproc imgcodecs)

if(OpenCV_FOUND)
    message(STATUS "OpenCV: ${OpenCV_VERSION} (${OpenCV_LIBS})")
else()
    message(FATAL_ERROR "OpenCV not found. Install: sudo apt install libopencv-dev")
endif()
