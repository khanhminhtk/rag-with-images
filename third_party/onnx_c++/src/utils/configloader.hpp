#ifndef ONNX_CONFIGLOADER_H
#define ONNX_CONFIGLOADER_H

#include <string>
#include <yaml-cpp/yaml.h>

#include "domain/value_objects/ConfigLoader.hpp"

class ConfigLoader {
public:
    explicit ConfigLoader(const std::string& pathfile);
    void TextEmbedding(const YAML::Node &config_node);
    [[nodiscard]] const TextEmbedConfig& GetTextEmbedConfig() const;
private:
    TextEmbedConfig text_embed_config_;
    template<typename T>
    static void loadCommonSession(const YAML::Node& node, T* config_current) {
        const auto& sessNode = node["session_config"];
        config_current->session_config.intra_op_num_threads = sessNode["intra_op_num_threads"].as<int>();
        config_current->session_config.inter_op_num_threads = sessNode["inter_op_num_threads"].as<int>();
        config_current->session_config.execution_mode = sessNode["execution_mode"].as<std::string>();
        config_current->session_config.graph_optimization_level = sessNode["graph_optimization_level"].as<std::string>();
        if (sessNode["log_severity_level"])
            config_current->session_config.log_severity_level = sessNode["log_severity_level"].as<std::string>();
        config_current->session_config.provider.type = sessNode["execution_provider"]["type"].as<std::string>();
        config_current->session_config.provider.device = sessNode["execution_provider"]["device"].as<int>();
        auto memNode = sessNode["memory_config"];
        config_current->session_config.memory.memory_pattern = (memNode["memory_pattern"].as<std::string>() == "enable");
        config_current->session_config.memory.cpu_mem_arena = (memNode["cpu_mem_arena"].as<std::string>() == "enable");
        if (memNode["arena_config"])
            config_current->session_config.memory.initial_chunk_size_bytes = memNode["arena_config"]["initial_chunk_size_bytes"].as<size_t>();
        else
            config_current->session_config.memory.initial_chunk_size_bytes = 0;
    }
};


#endif //ONNX_CONFIGLOADER_H
