#ifndef ONNX_CONFIGLOADER_HPP
#define ONNX_CONFIGLOADER_HPP

#include <cstddef>
#include <string>
#include <vector>

struct OnnxSessionConfig {
    int intra_op_num_threads = 4;
    int inter_op_num_threads = 2;
    std::string execution_mode        = "sequential";
    std::string graph_optimization_level = "enable_extended";
    std::string log_severity_level    = "warning";

    struct ExecutionProvider {
        std::string type   = "cpu";
        int         device = 0;
    } provider;

    struct MemoryConfig {
        bool   memory_pattern           = true;
        bool   cpu_mem_arena            = true;
        size_t initial_chunk_size_bytes = 1048576;
        size_t max_mem                  = 0;
        int    arena_extend_strategy    = 0;
    } memory;
};

struct TensorInfo {
    std::vector<std::string> shape;
    std::string              type;
};

struct TextEmbedInputs {
    TensorInfo input_ids;
    TensorInfo attention_mask;
};

struct TextEmbedConfig {
    std::string       model_path;
    std::string       vocab_path = "model/tokenizer/vocab.txt";
    TextEmbedInputs   inputs;
    TensorInfo        outputs;
    OnnxSessionConfig session_config;
};

struct VisionEmbedInputs {
    TensorInfo pixel_values;
};

struct VisionEmbedConfig {
    std::string        model_path;
    VisionEmbedInputs  inputs;
    TensorInfo         outputs;
    OnnxSessionConfig  session_config;
    int image_width  = 224;
    int image_height = 224;
};

#endif
