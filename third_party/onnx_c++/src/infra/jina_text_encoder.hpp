#pragma once

#include "application/port/IEncoder.hpp"
#include "application/port/IOnnxSession.hpp"

#include <string>
#include <vector>
#include <unordered_map>

class JinaTextEncoder : public ITextEncoder {
public:
    explicit JinaTextEncoder(IOnnxSession* session, const std::string& vocabPath = "model/tokenizer/vocab.txt");

    EmbeddingResult encode(const std::string& text) override;
    void setMaxSeqLen(int maxLen);

private:
    struct TokenizeResult {
        std::vector<int64_t> input_ids;
        std::vector<int64_t> attention_mask;
    };
    void loadVocab(const std::string& vocabPath);
    TokenizeResult tokenize(const std::string& text) const;

    IOnnxSession* session_;  // non-owning
    int maxSeqLen_ = 512;
    int embeddingDim_ = 768;

    std::unordered_map<std::string, int> vocab_;
    int unkTokenId_ = 100;

    std::vector<std::string> inputNamesStorage_;
    std::vector<std::string> outputNamesStorage_;
    std::vector<const char*> inputNames_;
    std::vector<const char*> outputNames_;
};
