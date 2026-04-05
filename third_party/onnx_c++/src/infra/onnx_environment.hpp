#pragma once

#include "onnxruntime_cxx_api.h"
#include "application/port/IOnnxEnvironment.hpp"

#include <memory>
#include <mutex>
#include <string>

class OnnxEnvironment : public IOnnxEnvironment {
public:
    explicit OnnxEnvironment(const std::string& logId,
                             OrtLoggingLevel level = ORT_LOGGING_LEVEL_WARNING,
                             bool debugMode = false);

    void* getNativeEnv() override;

private:
    struct LogParams {
        std::string logId;
        bool        debugMode;
        std::mutex  mutex;
    };

    static void ORT_API_CALL LogCallback(
        void* param, OrtLoggingLevel severity,
        const char* category, const char* logid,
        const char* code_location, const char* message);

    LogParams logParams_;
    std::unique_ptr<Ort::Env> env_;
};
