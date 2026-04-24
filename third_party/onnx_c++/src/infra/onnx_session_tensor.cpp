#include "onnx_session.hpp"

#include <stdexcept>

void* OnnxSession::createFloatTensor(const std::vector<int64_t>& shape, float* data) {
    if (!data)         throw std::invalid_argument("Data pointer cannot be null.");
    if (shape.empty()) throw std::invalid_argument("Shape cannot be empty.");

    size_t totalElements = 1;
    for (const auto dim : shape) totalElements *= static_cast<size_t>(dim);

    return static_cast<void*>(new Ort::Value(Ort::Value::CreateTensor<float>(
        memory_info_cpu_, data, totalElements, shape.data(), shape.size()
    )));
}

void* OnnxSession::createInt64Tensor(const std::vector<int64_t>& shape, int64_t* data) {
    if (!data)         throw std::invalid_argument("Data pointer cannot be null.");
    if (shape.empty()) throw std::invalid_argument("Shape cannot be empty.");

    size_t totalElements = 1;
    for (const auto dim : shape) totalElements *= static_cast<size_t>(dim);

    return static_cast<void*>(new Ort::Value(Ort::Value::CreateTensor<int64_t>(
        memory_info_cpu_, data, totalElements, shape.data(), shape.size()
    )));
}

void OnnxSession::releaseTensor(void* tensor) {
    if (tensor) delete static_cast<Ort::Value*>(tensor);
}

float* OnnxSession::getFloatTensorData(void* tensor) {
    if (!tensor) throw std::invalid_argument("Tensor pointer cannot be null.");
    auto* val = static_cast<Ort::Value*>(tensor);
    if (!val->IsTensor()) throw std::runtime_error("Value is not a tensor.");
    return val->GetTensorMutableData<float>();
}

int64_t* OnnxSession::getInt64TensorData(void* tensor) {
    if (!tensor) throw std::invalid_argument("Tensor pointer cannot be null.");
    auto* val = static_cast<Ort::Value*>(tensor);
    if (!val->IsTensor()) throw std::runtime_error("Value is not a tensor.");
    return val->GetTensorMutableData<int64_t>();
}

std::vector<int64_t> OnnxSession::getTensorShape(void* tensor) {
    if (!tensor) throw std::invalid_argument("Tensor pointer cannot be null.");
    const auto* val = static_cast<Ort::Value*>(tensor);
    if (!val->IsTensor()) throw std::runtime_error("Value is not a tensor.");
    return val->GetTensorTypeAndShapeInfo().GetShape();
}

size_t OnnxSession::getTensorElementCount(void* tensor) {
    if (!tensor) throw std::invalid_argument("Tensor pointer cannot be null.");
    const auto* val = static_cast<Ort::Value*>(tensor);
    if (!val->IsTensor()) throw std::runtime_error("Value is not a tensor.");
    return val->GetTensorTypeAndShapeInfo().GetElementCount();
}
