#pragma once

#include "container.hpp"
#include <string>

namespace bootstrap {

class ServiceRegistry {
public:
    static void registerServices(DIContainer& container, const std::string& configPath);

private:
    static void registerOnnxEnvironment(DIContainer& container);
    static void registerConfigLoaders(DIContainer& container, const std::string& configPath);
    static void registerTextEncoder(DIContainer& container);
    static void registerVisionEncoder(DIContainer& container);
};

}  // namespace bootstrap
