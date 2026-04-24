#ifndef ONNX_CONFIGLOADER_H
#define ONNX_CONFIGLOADER_H

#include <string>
#include <yaml-cpp/yaml.h>

#include "domain/value_objects/ConfigLoader.hpp"

class ConfigLoader {
public:
    explicit ConfigLoader(const std::string& pathfile);

    [[nodiscard]] const TextEmbedConfig&   GetTextEmbedConfig()   const;
    [[nodiscard]] const VisionEmbedConfig& GetVisionEmbedConfig() const;

private:
    void parseTextEmbedding(const YAML::Node& config_node);
    void parseVisionEmbedding(const YAML::Node& config_node);

    template<typename T>
    static void loadCommonSession(const YAML::Node& node, T* cfg) {
        const auto& sess = node["session_config"];
        cfg->session_config.intra_op_num_threads = sess["intra_op_num_threads"].as<int>();
        cfg->session_config.inter_op_num_threads = sess["inter_op_num_threads"].as<int>();
        cfg->session_config.execution_mode       = sess["execution_mode"].as<std::string>();
        cfg->session_config.graph_optimization_level = sess["graph_optimization_level"].as<std::string>();

        if (sess["log_severity_level"])
            cfg->session_config.log_severity_level = sess["log_severity_level"].as<std::string>();

        cfg->session_config.provider.type   = sess["execution_provider"]["type"].as<std::string>();
        cfg->session_config.provider.device = sess["execution_provider"]["device"].as<int>();

        auto mem = sess["memory_config"];
        cfg->session_config.memory.memory_pattern = (mem["memory_pattern"].as<std::string>() == "enable");
        cfg->session_config.memory.cpu_mem_arena  = (mem["cpu_mem_arena"].as<std::string>() == "enable");

        if (mem["arena_config"]) {
            cfg->session_config.memory.initial_chunk_size_bytes =
                mem["arena_config"]["initial_chunk_size_bytes"].as<size_t>();
            if (mem["arena_config"]["max_mem"])
                cfg->session_config.memory.max_mem = mem["arena_config"]["max_mem"].as<size_t>();
            if (mem["arena_config"]["arena_extend_strategy"])
                cfg->session_config.memory.arena_extend_strategy =
                    mem["arena_config"]["arena_extend_strategy"].as<int>();
        }
    }

    TextEmbedConfig   text_embed_config_;
    VisionEmbedConfig vision_embed_config_;
};

#endif
