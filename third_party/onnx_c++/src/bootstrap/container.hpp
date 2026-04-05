#pragma once

#include <functional>
#include <memory>
#include <mutex>
#include <stdexcept>
#include <tuple>
#include <typeindex>
#include <unordered_map>

namespace bootstrap {

class DIContainer {
public:
    template <typename Interface, typename Concrete, typename... Args>
    void registerType(Args&&... args) {
        std::lock_guard<std::mutex> lock(mtx_);
        const auto key = std::type_index(typeid(Interface));
        auto argsTuple = std::make_tuple(std::forward<Args>(args)...);
        registry_[key] = [argsTuple]() mutable {
            return std::static_pointer_cast<void>(
                std::apply([](auto&&... capturedArgs) {
                    return std::make_shared<Concrete>(
                        std::forward<decltype(capturedArgs)>(capturedArgs)...);
                }, argsTuple)
            );
        };
    }

    template <typename Interface, typename Concrete, typename... Args>
    void registerSingleton(Args&&... args) {
        std::lock_guard<std::mutex> lock(mtx_);
        auto instance = std::static_pointer_cast<void>(
            std::make_shared<Concrete>(std::forward<Args>(args)...)
        );
        const auto key = std::type_index(typeid(Interface));
        registry_[key] = [instance]() { return instance; };
    }

    template <typename Interface>
    void registerFactory(std::function<std::shared_ptr<Interface>()> factory) {
        std::lock_guard<std::mutex> lock(mtx_);
        const auto key = std::type_index(typeid(Interface));
        registry_[key] = [factory]() {
            return std::static_pointer_cast<void>(factory());
        };
    }

    template <typename Interface>
    std::shared_ptr<Interface> resolve() {
        const auto key = std::type_index(typeid(Interface));
        auto it = registry_.find(key);
        if (it == registry_.end()) {
            throw std::runtime_error(
                std::string("Service not registered: ") + typeid(Interface).name());
        }
        return std::static_pointer_cast<Interface>(it->second());
    }

private:
    std::unordered_map<std::type_index, std::function<std::shared_ptr<void>()>> registry_;
    mutable std::mutex mtx_;
};

}  // namespace bootstrap
