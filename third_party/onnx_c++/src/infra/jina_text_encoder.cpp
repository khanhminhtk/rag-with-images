#include "jina_text_encoder.hpp"

#include <iostream>
#include <stdexcept>
#include <fstream>
#include <algorithm>
#include <cctype>

JinaTextEncoder::JinaTextEncoder(IOnnxSession* session, const std::string& vocabPath) : session_(session) {
    if (!session_) throw std::invalid_argument("third_party.onnx_c++.src.infra.jina_text_encoder: IOnnxSession cannot be null");

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

    std::cout << "third_party.onnx_c++.src.infra.jina_text_encoder: Encoder initialized ("
              << numInputs << " inputs, " << numOutputs << " outputs)" << std::endl;
              
    loadVocab(vocabPath);
}

void JinaTextEncoder::loadVocab(const std::string& vocabPath) {
    std::ifstream file(vocabPath);
    if (!file.is_open()) {
        std::cerr << "Warning: Could not open vocab file at " << vocabPath << ". Using fallback tokenization.\n";
        return;
    }
    std::string line;
    int index = 0;
    while (std::getline(file, line)) {
        if (!line.empty() && line.back() == '\r') line.pop_back();
        vocab_[line] = index++;
    }
    auto it = vocab_.find("[UNK]");
    if (it != vocab_.end()) unkTokenId_ = it->second;
}

void JinaTextEncoder::setMaxSeqLen(const int maxLen) {
    if (maxLen <= 0) throw std::invalid_argument("third_party.onnx_c++.src.infra.jina_text_encoder: maxSeqLen must be positive");
    maxSeqLen_ = maxLen;
}

static bool isPunctuation(const int c) {
    if ((c >= 33 && c <= 47) || (c >= 58 && c <= 64) ||
        (c >= 91 && c <= 96) || (c >= 123 && c <= 126)) {
        return true;
    }
    return false;
}

JinaTextEncoder::TokenizeResult
JinaTextEncoder::tokenize(const std::string& text) const {
    TokenizeResult result;
    
    // Lowercase
    std::string lower_text = text;
    std::transform(lower_text.begin(), lower_text.end(), lower_text.begin(),
                   [](const unsigned char c){ return std::tolower(c); });

    // Basic Tokenize (split by whitespace and punctuation)
    std::vector<std::string> basic_tokens;
    std::string current_token;
    for (const char c : lower_text) {
        if (std::isspace(static_cast<unsigned char>(c))) {
            if (!current_token.empty()) {
                basic_tokens.push_back(current_token);
                current_token.clear();
            }
        } else if (isPunctuation(static_cast<unsigned char>(c))) {
            if (!current_token.empty()) {
                basic_tokens.push_back(current_token);
                current_token.clear();
            }
            basic_tokens.push_back(std::string(1, c));
        } else {
            current_token += c;
        }
    }
    if (!current_token.empty()) {
        basic_tokens.push_back(current_token);
    }

    // WordPiece Tokenize
    std::vector<int64_t> ids;
    ids.push_back(101); // [CLS]

    for (const auto& token : basic_tokens) {
        if (token.size() > 100) { // max_input_chars_per_word
            ids.push_back(unkTokenId_);
            continue;
        }
        
        bool is_bad = false;
        size_t start = 0;
        std::vector<int64_t> sub_tokens;
        
        while (start < token.length()) {
            size_t end = token.length();
            int cur_substr_id = -1;
            while (start < end) {
                std::string substr = token.substr(start, end - start);
                if (start > 0) {
                    substr = "##" + substr;
                }
                auto it = vocab_.find(substr);
                if (it != vocab_.end()) {
                    cur_substr_id = it->second;
                    break;
                }
                end -= 1;
            }
            if (cur_substr_id == -1) {
                is_bad = true;
                break;
            }
            sub_tokens.push_back(cur_substr_id);
            start = end;
        }
        if (is_bad) {
            ids.push_back(unkTokenId_);
        } else {
            ids.insert(ids.end(), sub_tokens.begin(), sub_tokens.end());
        }
    }

    // Truncate to maxSeqLen_ - 1 (to leave room for [SEP])
    if (ids.size() > static_cast<size_t>(maxSeqLen_ - 1)) {
        ids.resize(maxSeqLen_ - 1);
    }
    ids.push_back(102); // [SEP]

    // Pad
    result.input_ids = ids;
    result.attention_mask.assign(ids.size(), 1);
    
    if (result.input_ids.size() < static_cast<size_t>(maxSeqLen_)) {
        const size_t pad_len = maxSeqLen_ - result.input_ids.size();
        result.input_ids.insert(result.input_ids.end(), pad_len, 0);
        result.attention_mask.insert(result.attention_mask.end(), pad_len, 0);
    }

    return result;
}

EmbeddingResult JinaTextEncoder::encode(const std::string& text) {
    if (text.empty()) throw std::invalid_argument("third_party.onnx_c++.src.infra.jina_text_encoder: Input text cannot be empty");

    auto [input_ids, attention_mask] = tokenize(text);
    const int64_t seqLen = static_cast<int64_t>(input_ids.size());
    const std::vector<int64_t> inputShape = {1, seqLen};

    void* idsTensor  = session_->createInt64Tensor(inputShape, input_ids.data());
    void* maskTensor = session_->createInt64Tensor(inputShape, attention_mask.data());

    std::vector<void*> inputTensors;
    for (const auto& name : inputNamesStorage_) {
        if (name == "input_ids") {
            inputTensors.push_back(idsTensor);
        } else if (name == "attention_mask") {
            inputTensors.push_back(maskTensor);
        } else {
            // fallback if names are mapped differently
            inputTensors.push_back(idsTensor);
        }
    }
    
    if (inputTensors.size() != inputNames_.size()) {
         session_->releaseTensor(idsTensor);
         session_->releaseTensor(maskTensor);
         throw std::runtime_error("Tensors mapping mismatch");
    }

    std::vector<void*> outputTensors;

    session_->run(inputNames_, inputTensors, outputNames_, outputTensors);

    if (outputTensors.empty()) {
        session_->releaseTensor(idsTensor);
        session_->releaseTensor(maskTensor);
        throw std::runtime_error("third_party.onnx_c++.src.infra.jina_text_encoder: No output from text model");
    }

    void* outTensor = outputTensors[0];
    float* outData  = session_->getFloatTensorData(outTensor);
    const auto outShape   = session_->getTensorShape(outTensor);

    const int dim = (outShape.size() >= 2) ? static_cast<int>(outShape[1]) : embeddingDim_;
    std::vector<float> embedding(outData, outData + dim);

    session_->releaseTensor(idsTensor);
    session_->releaseTensor(maskTensor);
    for (auto* t : outputTensors) session_->releaseTensor(t);

    return EmbeddingResult(std::move(embedding), dim).normalized();
}
