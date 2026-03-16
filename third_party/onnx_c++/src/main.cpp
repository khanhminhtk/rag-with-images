#include <exception>
#include <iostream>
#include <string>

#include "utils/configloader.hpp"

namespace {

void PrintTensorInfo(const std::string& name, const TensorInfo& tensor) {
    std::cout << name << '\n';
    std::cout << "  type: " << tensor.type << '\n';
    std::cout << "  shape: [";
    for (std::size_t i = 0; i < tensor.shape.size(); ++i) {
        if (i > 0) {
            std::cout << ", ";
        }
        std::cout << tensor.shape[i];
    }
    std::cout << "]\n";
}

}  // namespace

int main(int argc, char* argv[]) {
    const std::string config_path =
        argc > 1 ? argv[1] : "/home/minhtk/code/rag_imtotext_texttoim/third_party/onnx_c++/config/config.yaml";

    try {
        const ConfigLoader loader(config_path);
        const TextEmbedConfig& config = loader.GetTextEmbedConfig();

        std::cout << "Loaded config: " << config_path << '\n';
        std::cout << "model_path: " << config.model_path << '\n';
        PrintTensorInfo("input_ids", config.inputs.input_ids);
        PrintTensorInfo("attention_mask", config.inputs.attention_mask);
        PrintTensorInfo("outputs", config.outputs);
        std::cout << "session_config\n";
        std::cout << "  intra_op_num_threads: " << config.session_config.intra_op_num_threads << '\n';
        std::cout << "  inter_op_num_threads: " << config.session_config.inter_op_num_threads << '\n';
        std::cout << "  execution_mode: " << config.session_config.execution_mode << '\n';
        std::cout << "  graph_optimization_level: " << config.session_config.graph_optimization_level << '\n';
        std::cout << "  log_severity_level: " << config.session_config.log_severity_level << '\n';
        std::cout << "  provider.type: " << config.session_config.provider.type << '\n';
        std::cout << "  provider.device: " << config.session_config.provider.device << '\n';
        std::cout << "  memory.memory_pattern: " << std::boolalpha
                  << config.session_config.memory.memory_pattern << '\n';
        std::cout << "  memory.cpu_mem_arena: " << std::boolalpha
                  << config.session_config.memory.cpu_mem_arena << '\n';
        std::cout << "  memory.initial_chunk_size_bytes: "
                  << config.session_config.memory.initial_chunk_size_bytes << '\n';
        return 0;
    } catch (const std::exception& ex) {
        std::cerr << "Failed to load config: " << ex.what() << '\n';
        return 1;
    }
}
