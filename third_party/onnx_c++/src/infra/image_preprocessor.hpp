#pragma once

#include <opencv2/opencv.hpp>
#include <vector>

class ImagePreprocessor {
public:
    // Resize + normalize [0,1] + HWC→CHW
    static std::vector<float> preprocess(const cv::Mat& image,
                                          int targetWidth  = 224,
                                          int targetHeight = 224);
    static void hwcToCwh(std::vector<float>& output, const cv::Mat& frame);
};
