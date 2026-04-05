#include "registry.hpp"

#include "infra/onnx_environment.hpp"
#include "infra/onnx_session.hpp"
#include "infra/jina_text_encoder.hpp"
#include "infra/jina_vision_encoder.hpp"
#include "utils/configloader.hpp"

#include <iostream>
#include <memory>

namespace bootstrap {

void ServiceRegistry::registerServices(DIContainer& container, const std::string& configPath) {
    registerConfigLoaders(container, configPath);
    registerOnnxEnvironment(container);
    registerTextEncoder(container);
    registerVisionEncoder(container);
}

void ServiceRegistry::registerConfigLoaders(DIContainer& container, const std::string& configPath) {
    container.registerSingleton<ConfigLoader, ConfigLoader>(configPath);
}

void ServiceRegistry::registerOnnxEnvironment(DIContainer& container) {
    container.registerSingleton<IOnnxEnvironment, OnnxEnvironment>(
        std::string("JinaClip"), ORT_LOGGING_LEVEL_WARNING, true
    );
}

void ServiceRegistry::registerTextEncoder(DIContainer& container) {
    auto env    = container.resolve<IOnnxEnvironment>();
    auto config = container.resolve<ConfigLoader>();
    const auto& textCfg = config->GetTextEmbedConfig();

    // OnnxSession owned by encoder lifecycle via DI container
    auto* session = new OnnxSession(env.get());
    session->initialize(textCfg.session_config);
    session->loadModel(textCfg.model_path.c_str());

    container.registerSingleton<JinaTextEncoder, JinaTextEncoder>(
        static_cast<IOnnxSession*>(session), textCfg.vocab_path
    );
}

void ServiceRegistry::registerVisionEncoder(DIContainer& container) {
    auto env    = container.resolve<IOnnxEnvironment>();
    auto config = container.resolve<ConfigLoader>();
    const auto& visCfg = config->GetVisionEmbedConfig();

    auto* session = new OnnxSession(env.get());
    session->initialize(visCfg.session_config);
    session->loadModel(visCfg.model_path.c_str());

    container.registerSingleton<JinaVisionEncoder, JinaVisionEncoder>(
        static_cast<IOnnxSession*>(session), visCfg.image_width, visCfg.image_height
    );
}

}  // namespace bootstrap
