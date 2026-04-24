#pragma once

#include "application/port/IOnnxEnvironment.hpp"
#include "application/port/IOnnxSession.hpp"
#include "onnxruntime_cxx_api.h"

#include <memory>
#include <string>
#include <vector>

class OnnxSession : public IOnnxSession {
public:
    explicit OnnxSession(IOnnxEnvironment* env);
    ~OnnxSession() override = default;

    void initialize(const OnnxSessionConfig& config) override;
    void loadModel(const char* modelPath) override;

    size_t getInputCount()  const override;
    size_t getOutputCount() const override;
    std::string getInputName(size_t index)  const override;
    std::string getOutputName(size_t index) const override;
    std::vector<int64_t> getInputShape(size_t index)  const override;
    std::vector<int64_t> getOutputShape(size_t index) const override;

    void* createFloatTensor(const std::vector<int64_t>& shape, float* data) override;
    void* createInt64Tensor(const std::vector<int64_t>& shape, int64_t* data) override;
    void  releaseTensor(void* tensor) override;

    float*   getFloatTensorData(void* tensor) override;
    int64_t* getInt64TensorData(void* tensor) override;
    std::vector<int64_t> getTensorShape(void* tensor) override;
    size_t getTensorElementCount(void* tensor) override;

    void run(
        const std::vector<const char*>& inputNames,
        const std::vector<void*>&       inputTensors,
        const std::vector<const char*>& outputNames,
        std::vector<void*>&             outputTensors
    ) override;

    void inference(const std::vector<float>& inputData,
                   const std::vector<int64_t>& inputShape,
                   std::vector<float>& outputBuffer,
                   const std::vector<int64_t>& outputShape) const override;

private:
    // Config helpers (onnx_session_config.cpp)
    void configureExecutionMode(const std::string& mode) const;
    void configureGraphOptimization(const std::string& level) const;
    void configureExecutionProvider(const OnnxSessionConfig::ExecutionProvider& provider) const;
    void configureMemory(const OnnxSessionConfig::MemoryConfig& memConfig) const;
    void configureLogLevel(const std::string& level) const;

    IOnnxEnvironment* env_;  // non-owning
    std::unique_ptr<Ort::SessionOptions> sessionOptions_;
    std::unique_ptr<Ort::Session>        session_;
    std::unique_ptr<Ort::IoBinding>      io_binding_;

    Ort::MemoryInfo memory_info_cpu_;

    std::vector<std::string>    input_names_storage_;
    std::vector<std::string>    output_names_storage_;
    std::vector<const char*>    input_names_;
    std::vector<const char*>    output_names_;
};
