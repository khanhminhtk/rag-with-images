#pragma once

#include <cstddef>
#include <cstdint>
#include <string>
#include <vector>

#include "domain/value_objects/ConfigLoader.hpp"

class IOnnxSession {
public:
    virtual ~IOnnxSession() = default;

    virtual void initialize(const OnnxSessionConfig& config) = 0;
    virtual void loadModel(const char* modelPath) = 0;

    virtual size_t getInputCount()  const = 0;
    virtual size_t getOutputCount() const = 0;
    virtual std::string getInputName(size_t index)  const = 0;
    virtual std::string getOutputName(size_t index) const = 0;
    virtual std::vector<int64_t> getInputShape(size_t index)  const = 0;
    virtual std::vector<int64_t> getOutputShape(size_t index) const = 0;

    virtual void* createFloatTensor(const std::vector<int64_t>& shape, float* data) = 0;
    virtual void* createInt64Tensor(const std::vector<int64_t>& shape, int64_t* data) = 0;
    virtual void  releaseTensor(void* tensor) = 0;

    virtual float*   getFloatTensorData(void* tensor) = 0;
    virtual int64_t* getInt64TensorData(void* tensor) = 0;
    virtual std::vector<int64_t> getTensorShape(void* tensor) = 0;
    virtual size_t getTensorElementCount(void* tensor) = 0;

    virtual void run(
        const std::vector<const char*>& inputNames,
        const std::vector<void*>&       inputTensors,
        const std::vector<const char*>& outputNames,
        std::vector<void*>&             outputTensors
    ) = 0;

    // Single-input single-output convenience with IoBinding
    virtual void inference(const std::vector<float>& inputData,
                           const std::vector<int64_t>& inputShape,
                           std::vector<float>& outputBuffer,
                           const std::vector<int64_t>& outputShape) const = 0;
};
