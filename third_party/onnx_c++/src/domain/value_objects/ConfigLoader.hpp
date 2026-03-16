//
// Created by minhtk on 15/03/2026.
//

#ifndef ONNX_CONFIGLOADER_HPP
#define ONNX_CONFIGLOADER_HPP
#include <cstddef>
#include <string>
#include <vector>

struct OnnxSessionConfig {
    int intra_op_num_threads;
    int inter_op_num_threads;
    std::string execution_mode;
    std::string graph_optimization_level;
    std::string log_severity_level;

    struct execution_provider {
        std::string type;
        int device;
    } provider;

    struct MemoryConfig {
        bool memory_pattern;
        bool cpu_mem_arena;
        size_t initial_chunk_size_bytes;
    } memory;
};

struct TensorInfo {
    std::vector<std::string> shape;
    std::string type;
};

struct TextEmbedInputs {
    TensorInfo input_ids;
    TensorInfo attention_mask;
};

struct TextEmbedConfig {
    std::string model_path;
    TextEmbedInputs inputs;
    TensorInfo outputs;
    OnnxSessionConfig session_config;
};

struct ImageEmbedConfig {
    std::string PathFile;
};

#endif //ONNX_CONFIGLOADER_HPP
