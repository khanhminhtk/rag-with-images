#include "bridge.h"
#include "../bootstrap/container.hpp"
#include "../bootstrap/registry.hpp"
#include "../infra/jina_text_encoder.hpp"
#include "../infra/jina_vision_encoder.hpp"
#include <opencv2/opencv.hpp>
#include <vector>
#include <string>
#include <cstring>
#include <exception>

extern "C" {

JinaHandle jina_init(const char* config_path) {
    try {
        if (!config_path) return nullptr;
        
        auto* container = new bootstrap::DIContainer();
        bootstrap::ServiceRegistry::registerServices(*container, config_path);
        return static_cast<JinaHandle>(container);
    } catch (...) {
        return nullptr;
    }
}

void jina_release(JinaHandle handle) {
    if (handle) {
        delete static_cast<bootstrap::DIContainer*>(handle);
    }
}

int jina_embed_text(JinaHandle handle, const char* text, float* out_data) {
    try {
        if (!handle || !text || !out_data) return -1;
        
        auto* container = static_cast<bootstrap::DIContainer*>(handle);
        auto encoder = container->resolve<JinaTextEncoder>();
        auto result = encoder->encode(text);
        
        std::memcpy(out_data, result.data().data(), result.dimension() * sizeof(float));
        return 0;
    } catch (const std::exception& e) {
        return -1;
    } catch (...) {
        return -2;
    }
}

int jina_embed_batch_text(JinaHandle handle, const char** texts, int count, float* out_data) {
    try {
        if (!handle || !texts || count <= 0 || !out_data) return -1;
        
        auto* container = static_cast<bootstrap::DIContainer*>(handle);
        auto encoder = container->resolve<JinaTextEncoder>();
        
        std::vector<std::string> batch;
        batch.reserve(count);
        for (int i = 0; i < count; ++i) {
            if (texts[i]) batch.push_back(texts[i]);
            else batch.push_back("");
        }
        
        auto results = encoder->encodeBatch(batch);
        for (int i = 0; i < count; ++i) {
            std::memcpy(out_data + i * 768, results[i].data().data(), 768 * sizeof(float));
        }
        return 0;
    } catch (...) { return -1; }
}

int jina_embed_image(JinaHandle handle, const uint8_t* img_data, int width, int height, int channels, float* out_data) {
    try {
        if (!handle || !img_data || !out_data) return -1;
        
        auto* container = static_cast<bootstrap::DIContainer*>(handle);
        auto encoder = container->resolve<JinaVisionEncoder>();
        
        // Wrap raw buffer in cv::Mat (OpenCV doesn't take ownership)
        cv::Mat img(height, width, CV_8UC(channels), (void*)img_data);
        auto result = encoder->encodeFromMat(img);
        
        std::memcpy(out_data, result.data().data(), 768 * sizeof(float));
        return 0;
    } catch (...) { return -1; }
}

int jina_embed_batch_image(JinaHandle handle, const uint8_t* imgs_data, int count, int width, int height, int channels, float* out_data) {
    try {
        if (!handle || !imgs_data || count <= 0 || !out_data) return -1;
        
        auto* container = static_cast<bootstrap::DIContainer*>(handle);
        auto encoder = container->resolve<JinaVisionEncoder>();
        
        std::vector<cv::Mat> batch;
        batch.reserve(count);
        size_t frame_size = static_cast<size_t>(width * height * channels);
        
        for (int i = 0; i < count; ++i) {
            // We clone because the underlying buffer imgs_data is owned by Go/Caller
            // and might be mutated or released after this call. 
            // JinaVisionEncoder::encodeBatchFromMat takes a vector of Mats.
            cv::Mat img(height, width, CV_8UC(channels), (void*)(imgs_data + i * frame_size));
            batch.push_back(img.clone());
        }
        
        auto results = encoder->encodeBatchFromMat(batch);
        for (int i = 0; i < count; ++i) {
            std::memcpy(out_data + i * 768, results[i].data().data(), 768 * sizeof(float));
        }
        return 0;
    } catch (...) { return -1; }
}

}
