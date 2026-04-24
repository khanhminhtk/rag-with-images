cmake_minimum_required(VERSION 3.10)
project(EdgeSentinel VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# ── Domain layer ────────────────────────────────
set(DOMAIN_HEADERS
    domain/value_objects/ConfigLoader.hpp
    domain/value_objects/EmbeddingResult.hpp
)

# ── Application layer (ports / interfaces) ──────
set(APPLICATION_HEADERS
    application/port/ILogger.hpp
    application/port/IOnnxEnvironment.hpp
    application/port/IOnnxSession.hpp
    application/port/IEncoder.hpp
)

# ── Infrastructure layer ────────────────────────
set(INFRA_SOURCES
    infra/onnx_environment.cpp
    infra/onnx_session.cpp
    infra/onnx_session_config.cpp
    infra/onnx_session_tensor.cpp
    infra/image_preprocessor.cpp
    infra/jina_text_encoder.cpp
    infra/jina_vision_encoder.cpp
)

set(INFRA_HEADERS
    infra/onnx_environment.hpp
    infra/onnx_session.hpp
    infra/image_preprocessor.hpp
    infra/jina_text_encoder.hpp
    infra/jina_vision_encoder.hpp
)

# ── Bootstrap (DI container) ───────────────────
set(BOOTSTRAP_SOURCES
    bootstrap/registry.cpp
)

set(BOOTSTRAP_HEADERS
    bootstrap/container.hpp
    bootstrap/registry.hpp
)

# ── Utils ───────────────────────────────────────
set(UTILS_SOURCES
    utils/configloader.cpp
)

set(UTILS_HEADERS
    utils/configloader.hpp
    utils/spdlogger.hpp
)

# ── API layer (CGO Bridge) ──────────────────────
set(API_SOURCES
    api/bridge.cpp
)

set(API_HEADERS
    api/bridge.h
)

# ── Static library ──────────────────────────────
add_library(edge_sentinel_lib STATIC
    ${INFRA_SOURCES}
    ${BOOTSTRAP_SOURCES}
    ${UTILS_SOURCES}
    ${API_SOURCES}
    ${DOMAIN_HEADERS}
    ${APPLICATION_HEADERS}
    ${INFRA_HEADERS}
    ${BOOTSTRAP_HEADERS}
    ${UTILS_HEADERS}
    ${API_HEADERS}
)

target_include_directories(edge_sentinel_lib
    PUBLIC
        $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}>
        $<INSTALL_INTERFACE:include>
)

target_link_libraries(edge_sentinel_lib
    PUBLIC
        onnxruntime
        spdlog::spdlog
        yaml-cpp
        ${OpenCV_LIBS}
)

if(NOT MSVC)
    target_compile_options(edge_sentinel_lib PRIVATE -O2 -Wall -Wextra)
endif()

if(COMMAND set_project_warnings)
    set_project_warnings(edge_sentinel_lib)
endif()

# ── Executable ──────────────────────────────────
add_executable(edge_sentinel main.cpp)

target_link_libraries(edge_sentinel
    PRIVATE
        edge_sentinel_lib
)
