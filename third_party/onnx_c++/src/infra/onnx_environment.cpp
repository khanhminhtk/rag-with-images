#include "onnx_environment.hpp"

#include <iostream>
#include <string>

OnnxEnvironment::OnnxEnvironment(const std::string& logId,
                                 OrtLoggingLevel level,
                                 const bool debugMode) {
    logParams_.logId     = logId;
    logParams_.debugMode = debugMode;

    env_ = std::make_unique<Ort::Env>(
        level, logId.c_str(),
        OnnxEnvironment::LogCallback, &logParams_
    );
}

void* OnnxEnvironment::getNativeEnv() {
    if (!env_) {
        throw std::runtime_error("ONNX Runtime environment not initialized.");
    }
    return env_.get();
}

void ORT_API_CALL OnnxEnvironment::LogCallback(
    void* param, const OrtLoggingLevel severity,
    const char*, const char* logid,
    const char*, const char* message
) {
    auto* ctx = static_cast<LogParams*>(param);
    std::lock_guard<std::mutex> lock(ctx->mutex);

    auto levelStr = "[UNKNOWN]";
    switch (severity) {
        case ORT_LOGGING_LEVEL_VERBOSE: levelStr = "[VERBOSE]"; break;
        case ORT_LOGGING_LEVEL_INFO:    levelStr = "[INFO]";    break;
        case ORT_LOGGING_LEVEL_WARNING: levelStr = "[WARN]";    break;
        case ORT_LOGGING_LEVEL_ERROR:   levelStr = "[ERROR]";   break;
        case ORT_LOGGING_LEVEL_FATAL:   levelStr = "[FATAL]";   break;
    }

    if (ctx->debugMode) {
        auto& out = (severity >= ORT_LOGGING_LEVEL_ERROR) ? std::cerr : std::cout;
        out << "[" << ctx->logId << "]" << levelStr
            << "[" << logid << "] " << message << std::endl;
    }
}
