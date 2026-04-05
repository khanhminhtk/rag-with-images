#pragma once

#include <string>
#include <vector>

#include "domain/value_objects/EmbeddingResult.hpp"

class ITextEncoder {
public:
    virtual ~ITextEncoder() = default;
    virtual EmbeddingResult encode(const std::string& text) = 0;
};

class IVisionEncoder {
public:
    virtual ~IVisionEncoder() = default;
    // imageData: CHW float32 normalised [0,1]
    virtual EmbeddingResult encode(const std::vector<float>& imageData,
                                   int height, int width) = 0;
};
