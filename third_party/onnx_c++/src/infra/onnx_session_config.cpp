#include "onnx_session.hpp"

#include <iostream>
#include <stdexcept>

void OnnxSession::configureExecutionMode(const std::string& mode) const {
    if (mode == "parallel") {
        sessionOptions_->SetExecutionMode(ExecutionMode::ORT_PARALLEL);
    } else if (mode == "sequential") {
        sessionOptions_->SetExecutionMode(ExecutionMode::ORT_SEQUENTIAL);
    } else {
        throw std::invalid_argument("Unknown execution_mode: " + mode);
    }
}

void OnnxSession::configureGraphOptimization(const std::string& level) const {
    if (level == "disable_all") {
        sessionOptions_->SetGraphOptimizationLevel(GraphOptimizationLevel::ORT_DISABLE_ALL);
    } else if (level == "enable_basic") {
        sessionOptions_->SetGraphOptimizationLevel(GraphOptimizationLevel::ORT_ENABLE_BASIC);
    } else if (level == "enable_extended") {
        sessionOptions_->SetGraphOptimizationLevel(GraphOptimizationLevel::ORT_ENABLE_EXTENDED);
    } else if (level == "enable_all") {
        sessionOptions_->SetGraphOptimizationLevel(GraphOptimizationLevel::ORT_ENABLE_ALL);
    } else {
        sessionOptions_->SetGraphOptimizationLevel(GraphOptimizationLevel::ORT_ENABLE_EXTENDED);
    }
}

void OnnxSession::configureExecutionProvider(
    const OnnxSessionConfig::ExecutionProvider& provider
) const {
    if (provider.type == "cpu") {
        std::cout << "[INFO] Using CPU execution provider" << std::endl;
    } else if (provider.type == "cuda") {
        OrtCUDAProviderOptions cuda_opts;
        cuda_opts.device_id = provider.device;
        sessionOptions_->AppendExecutionProvider_CUDA(cuda_opts);
    } else if (provider.type == "tensorrt") {
        OrtTensorRTProviderOptionsV2* trt_opts = nullptr;
        auto& api = Ort::GetApi();
        Ort::ThrowOnError(api.CreateTensorRTProviderOptions(&trt_opts));
        try {
            std::string device_str = std::to_string(provider.device);
            const char* keys[]   = {"device_id"};
            const char* values[] = {device_str.c_str()};
            Ort::ThrowOnError(api.UpdateTensorRTProviderOptions(trt_opts, keys, values, 1));
            Ort::ThrowOnError(api.SessionOptionsAppendExecutionProvider_TensorRT_V2(
                sessionOptions_->operator OrtSessionOptions*(), trt_opts));
            api.ReleaseTensorRTProviderOptions(trt_opts);
        } catch (...) {
            api.ReleaseTensorRTProviderOptions(trt_opts);
            throw;
        }
    } else {
        std::cout << "[WARN] Unknown provider '" << provider.type << "', falling back to CPU" << std::endl;
    }
}

void OnnxSession::configureMemory(const OnnxSessionConfig::MemoryConfig& memConfig) const {
    if (memConfig.memory_pattern) sessionOptions_->EnableMemPattern();
    else                          sessionOptions_->DisableMemPattern();

    if (memConfig.cpu_mem_arena) sessionOptions_->EnableCpuMemArena();
    else                         sessionOptions_->DisableCpuMemArena();
}

void OnnxSession::configureLogLevel(const std::string& level) const {
    int severity = 2;
    if (level == "verbose")      severity = 0;
    else if (level == "info")    severity = 1;
    else if (level == "warning") severity = 2;
    else if (level == "error")   severity = 3;
    else if (level == "fatal")   severity = 4;
    sessionOptions_->SetLogSeverityLevel(severity);
}
