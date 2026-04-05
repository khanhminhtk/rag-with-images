#include "configloader.hpp"

ConfigLoader::ConfigLoader(const std::string& pathfile) {
    const YAML::Node config = YAML::LoadFile(pathfile);
    if (config["models"]["jina_text_encoder"])   parseTextEmbedding(config);
    if (config["models"]["jina_vision_encoder"]) parseVisionEmbedding(config);
}

void ConfigLoader::parseTextEmbedding(const YAML::Node& config_node) {
    auto root = config_node["models"]["jina_text_encoder"];

    text_embed_config_.model_path = root["model_path"].as<std::string>();
    if (root["vocab_path"]) {
        text_embed_config_.vocab_path = root["vocab_path"].as<std::string>();
    }

    text_embed_config_.inputs.input_ids.shape =
        root["inputs"]["input_ids"]["shape"].as<std::vector<std::string>>();
    text_embed_config_.inputs.input_ids.type =
        root["inputs"]["input_ids"]["type"].as<std::string>();

    text_embed_config_.inputs.attention_mask.shape =
        root["inputs"]["attention_mask"]["shape"].as<std::vector<std::string>>();
    text_embed_config_.inputs.attention_mask.type =
        root["inputs"]["attention_mask"]["type"].as<std::string>();

    text_embed_config_.outputs.shape = root["outputs"]["shape"].as<std::vector<std::string>>();
    text_embed_config_.outputs.type  = root["outputs"]["type"].as<std::string>();

    loadCommonSession<TextEmbedConfig>(root, &text_embed_config_);
}

void ConfigLoader::parseVisionEmbedding(const YAML::Node& config_node) {
    auto root = config_node["models"]["jina_vision_encoder"];

    vision_embed_config_.model_path = root["model_path"].as<std::string>();

    vision_embed_config_.inputs.pixel_values.shape =
        root["inputs"]["pixel_values"]["shape"].as<std::vector<std::string>>();
    vision_embed_config_.inputs.pixel_values.type =
        root["inputs"]["pixel_values"]["type"].as<std::string>();

    vision_embed_config_.outputs.shape = root["outputs"]["shape"].as<std::vector<std::string>>();
    vision_embed_config_.outputs.type  = root["outputs"]["type"].as<std::string>();

    if (root["image_width"])  vision_embed_config_.image_width  = root["image_width"].as<int>();
    if (root["image_height"]) vision_embed_config_.image_height = root["image_height"].as<int>();

    loadCommonSession<VisionEmbedConfig>(root, &vision_embed_config_);
}

const TextEmbedConfig& ConfigLoader::GetTextEmbedConfig() const {
    return text_embed_config_;
}

const VisionEmbedConfig& ConfigLoader::GetVisionEmbedConfig() const {
    return vision_embed_config_;
}
