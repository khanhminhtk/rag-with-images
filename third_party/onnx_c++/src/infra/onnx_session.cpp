#include "onnx_session.hpp"

#include <iostream>
#include <stdexcept>

OnnxSession::OnnxSession(IOnnxEnvironment* env)
    : env_(env),
      memory_info_cpu_(Ort::MemoryInfo::CreateCpu(
          OrtAllocatorType::OrtArenaAllocator,
          OrtMemType::OrtMemTypeDefault)) {
    if (!env_) {
        throw std::invalid_argument("OnnxEnvironment cannot be null");
    }
}

void OnnxSession::initialize(const OnnxSessionConfig& config) {
    sessionOptions_ = std::make_unique<Ort::SessionOptions>();
    sessionOptions_->SetIntraOpNumThreads(config.intra_op_num_threads);
    sessionOptions_->SetInterOpNumThreads(config.inter_op_num_threads);
    configureExecutionMode(config.execution_mode);
    configureGraphOptimization(config.graph_optimization_level);
    configureExecutionProvider(config.provider);
    configureMemory(config.memory);
    configureLogLevel(config.log_severity_level);
}

void OnnxSession::loadModel(const char* modelPath) {
    if (!sessionOptions_) {
        throw std::runtime_error("Call initialize() first.");
    }
    if (!modelPath) {
        throw std::invalid_argument("Model path cannot be null.");
    }

    auto* ortEnv = static_cast<Ort::Env*>(env_->getNativeEnv());
    session_ = std::make_unique<Ort::Session>(*ortEnv, modelPath, *sessionOptions_);
    io_binding_ = std::make_unique<Ort::IoBinding>(*session_);

    const Ort::AllocatorWithDefaultOptions allocator;
    const size_t input_count  = session_->GetInputCount();
    const size_t output_count = session_->GetOutputCount();

    input_names_storage_.clear();
    output_names_storage_.clear();
    input_names_.clear();
    output_names_.clear();

    for (size_t i = 0; i < input_count; ++i) {
        auto name = session_->GetInputNameAllocated(i, allocator);
        input_names_storage_.emplace_back(name.get());
        input_names_.push_back(input_names_storage_.back().c_str());
    }
    for (size_t i = 0; i < output_count; ++i) {
        auto name = session_->GetOutputNameAllocated(i, allocator);
        output_names_storage_.emplace_back(name.get());
        output_names_.push_back(output_names_storage_.back().c_str());
    }

    std::cout << "[INFO] Model loaded: " << modelPath
              << " (" << input_count << " inputs, " << output_count << " outputs)"
              << std::endl;
}

size_t OnnxSession::getInputCount() const {
    if (!session_) throw std::runtime_error("Session not loaded.");
    return session_->GetInputCount();
}

size_t OnnxSession::getOutputCount() const {
    if (!session_) throw std::runtime_error("Session not loaded.");
    return session_->GetOutputCount();
}

std::string OnnxSession::getInputName(const size_t index) const {
    if (!session_) throw std::runtime_error("Session not loaded.");
    const Ort::AllocatorWithDefaultOptions allocator;
    const auto name = session_->GetInputNameAllocated(index, allocator);
    return {name.get()};
}

std::string OnnxSession::getOutputName(const size_t index) const {
    if (!session_) throw std::runtime_error("Session not loaded.");
    const Ort::AllocatorWithDefaultOptions allocator;
    const auto name = session_->GetOutputNameAllocated(index, allocator);
    return {name.get()};
}

std::vector<int64_t> OnnxSession::getInputShape(const size_t index) const {
    if (!session_) throw std::runtime_error("Session not loaded.");
    return session_->GetInputTypeInfo(index).GetTensorTypeAndShapeInfo().GetShape();
}

std::vector<int64_t> OnnxSession::getOutputShape(const size_t index) const {
    if (!session_) throw std::runtime_error("Session not loaded.");
    return session_->GetOutputTypeInfo(index).GetTensorTypeAndShapeInfo().GetShape();
}

void OnnxSession::run(
    const std::vector<const char*>& inputNames,
    const std::vector<void*>&       inputTensors,
    const std::vector<const char*>& outputNames,
    std::vector<void*>&             outputTensors
) {
    if (!session_) throw std::runtime_error("Session not loaded.");
    if (inputNames.size() != inputTensors.size()) {
        throw std::invalid_argument("Input names and tensors size mismatch.");
    }

    std::vector<Ort::Value> ortInputs;
    ortInputs.reserve(inputTensors.size());
    for (auto* ptr : inputTensors) {
        ortInputs.push_back(std::move(*static_cast<Ort::Value*>(ptr)));
    }

    const Ort::RunOptions runOptions;
    auto ortOutputs = session_->Run(
        runOptions,
        inputNames.data(), ortInputs.data(), inputNames.size(),
        outputNames.data(), outputNames.size()
    );

    // Wrap outputs as heap-allocated Ort::Value (caller must releaseTensor)
    outputTensors.clear();
    outputTensors.reserve(ortOutputs.size());
    for (auto& tensor : ortOutputs) {
        outputTensors.push_back(static_cast<void*>(new Ort::Value(std::move(tensor))));
    }
}

void OnnxSession::inference(
    const std::vector<float>& inputData,
    const std::vector<int64_t>& inputShape,
    std::vector<float>& outputBuffer,
    const std::vector<int64_t>& outputShape
) const {
    if (!session_ || !io_binding_) throw std::runtime_error("Session not loaded.");

    io_binding_->ClearBoundInputs();
    io_binding_->ClearBoundOutputs();

    const Ort::Value inputTensor = Ort::Value::CreateTensor<float>(
        memory_info_cpu_,
        const_cast<float*>(inputData.data()), inputData.size(),
        inputShape.data(), inputShape.size()
    );
    io_binding_->BindInput(input_names_[0], inputTensor);

    const Ort::Value outputTensor = Ort::Value::CreateTensor<float>(
        memory_info_cpu_,
        outputBuffer.data(), outputBuffer.size(),
        outputShape.data(), outputShape.size()
    );
    io_binding_->BindOutput(output_names_[0], outputTensor);

    session_->Run(Ort::RunOptions{nullptr}, *io_binding_);
}
