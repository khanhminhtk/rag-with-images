#pragma once

#include "application/port/IEncoder.hpp"
#include "application/port/IOnnxSession.hpp"

#include <opencv2/opencv.hpp>
#include <string>
#include <vector>

class JinaVisionEncoder : public IVisionEncoder {
public:
    explicit JinaVisionEncoder(IOnnxSession* session,
                                int imageWidth  = 224,
                                int imageHeight = 224);

    EmbeddingResult encode(const std::vector<float>& imageData,
                           int height, int width) override;

    EmbeddingResult encodeFromMat(const cv::Mat& image);

private:
    IOnnxSession* session_;  // non-owning
    int imageWidth_;
    int imageHeight_;
    int embeddingDim_ = 768;

    std::vector<std::string> inputNamesStorage_;
    std::vector<std::string> outputNamesStorage_;
    std::vector<const char*> inputNames_;
    std::vector<const char*> outputNames_;
};
