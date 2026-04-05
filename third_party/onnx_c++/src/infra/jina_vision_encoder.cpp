#include "jina_vision_encoder.hpp"
#include "image_preprocessor.hpp"

#include <iostream>
#include <stdexcept>

JinaVisionEncoder::JinaVisionEncoder(IOnnxSession* session,
                                       const int imageWidth, const int imageHeight)
    : session_(session), imageWidth_(imageWidth), imageHeight_(imageHeight) {
    if (!session_) throw std::invalid_argument("third_party.onnx_c++.src.infra.jina_vision_encoder: IOnnxSession cannot be null");

    const size_t numInputs  = session_->getInputCount();
    const size_t numOutputs = session_->getOutputCount();

    for (size_t i = 0; i < numInputs; ++i) {
        inputNamesStorage_.push_back(session_->getInputName(i));
        inputNames_.push_back(inputNamesStorage_.back().c_str());
    }
    for (size_t i = 0; i < numOutputs; ++i) {
        outputNamesStorage_.push_back(session_->getOutputName(i));
        outputNames_.push_back(outputNamesStorage_.back().c_str());
    }

    std::cout << "third_party.onnx_c++.src.infra.jina_vision_encoder: Encoder initialized ("
              << numInputs << " inputs, " << numOutputs << " outputs, "
              << imageWidth_ << "x" << imageHeight_ << ")" << std::endl;
}

EmbeddingResult JinaVisionEncoder::encode(
    const std::vector<float>& imageData, const int height, const int width
) {
    const size_t expected = static_cast<size_t>(3) *
                            static_cast<size_t>(height) *
                            static_cast<size_t>(width);
    if (imageData.size() != expected) {
        throw std::invalid_argument(
            "third_party.onnx_c++.src.infra.jina_vision_encoder: Image data size mismatch: expected " + std::to_string(expected) +
            " got " + std::to_string(imageData.size()));
    }

    // Input shape: [batch=1, channels=3, H, W]
    const std::vector<int64_t> inputShape = {1, 3,
                                        static_cast<int64_t>(height),
                                        static_cast<int64_t>(width)};

    std::vector<float> mutableData = imageData;
    void* inputTensor = session_->createFloatTensor(inputShape, mutableData.data());

    const std::vector<void*> inputTensors  = {inputTensor};
    std::vector<void*> outputTensors;

    session_->run(inputNames_, inputTensors, outputNames_, outputTensors);

    if (outputTensors.empty()) {
        session_->releaseTensor(inputTensor);
        throw std::runtime_error("third_party.onnx_c++.src.infra.jina_vision_encoder: No output from vision model");
    }

    void* outTensor = outputTensors[0];
    float* outData  = session_->getFloatTensorData(outTensor);
    const auto outShape   = session_->getTensorShape(outTensor);

    const int dim = (outShape.size() >= 2) ? static_cast<int>(outShape[1]) : embeddingDim_;
    std::vector<float> embedding(outData, outData + dim);

    session_->releaseTensor(inputTensor);
    for (auto* t : outputTensors) session_->releaseTensor(t);

    return EmbeddingResult(std::move(embedding), dim).normalized();
}

EmbeddingResult JinaVisionEncoder::encodeFromMat(const cv::Mat& image) {
    const auto chwData = ImagePreprocessor::preprocess(image, imageWidth_, imageHeight_);
    return encode(chwData, imageHeight_, imageWidth_);
}
