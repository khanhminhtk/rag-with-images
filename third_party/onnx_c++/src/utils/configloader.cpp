//
// Created by minhtk on 15/03/2026.
//

#include <yaml-cpp/yaml.h>

#include "configloader.hpp"
#include "domain/value_objects/ConfigLoader.hpp"


ConfigLoader::ConfigLoader(const std::string &pathfile)  {
    const YAML::Node config = YAML::LoadFile(pathfile);
    TextEmbedding(config);
} ;

void ConfigLoader::TextEmbedding(const YAML::Node &config_node) {
    auto root = config_node["models"]["jina_text_encoder"];
    text_embed_config_.model_path = root["model_path"].as<std::string>();
    text_embed_config_.inputs.input_ids.shape = root["inputs"]["input_ids"]["shape"].as<std::vector<std::string>>();
    text_embed_config_.inputs.input_ids.type = root["inputs"]["input_ids"]["type"].as<std::string>();
    text_embed_config_.inputs.attention_mask.shape = root["inputs"]["attention_mask"]["shape"].as<std::vector<std::string>>();
    text_embed_config_.inputs.attention_mask.type = root["inputs"]["attention_mask"]["type"].as<std::string>();
    text_embed_config_.outputs.shape = root["outputs"]["shape"].as<std::vector<std::string>>();
    text_embed_config_.outputs.type = root["outputs"]["type"].as<std::string>();
    loadCommonSession<TextEmbedConfig>(root, &text_embed_config_);
}

const TextEmbedConfig& ConfigLoader::GetTextEmbedConfig() const {
    return text_embed_config_;
}

