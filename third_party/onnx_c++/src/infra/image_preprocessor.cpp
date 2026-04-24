#include "image_preprocessor.hpp"

#include <cstring>
#include <stdexcept>

std::vector<float> ImagePreprocessor::preprocess(
    const cv::Mat& image, const int targetWidth, const int targetHeight
) {
    if (image.empty()) throw std::invalid_argument("third_party.onnx_c++.src.infra.image_preprocessor: Input image is empty");

    cv::Mat resized;
    if (image.cols != targetWidth || image.rows != targetHeight) {
        cv::resize(image, resized, cv::Size(targetWidth, targetHeight));
    } else {
        resized = image;
    }

    cv::Mat rgb;
    cv::cvtColor(resized, rgb, cv::COLOR_BGR2RGB);

    cv::Mat floatImg;
    rgb.convertTo(floatImg, CV_32FC3, 1.0f / 255.0f);

    std::vector<float> output(static_cast<size_t>(floatImg.total()) *
                              static_cast<size_t>(floatImg.channels()));
    hwcToCwh(output, floatImg);
    return output;
}

void ImagePreprocessor::hwcToCwh(std::vector<float>& output, const cv::Mat& frame) {
    if (frame.empty()) return;

    const int channel_area   = frame.cols * frame.rows;
    const int channels_count = frame.channels();

    std::vector<cv::Mat> channels(static_cast<size_t>(channels_count));
    cv::split(frame, channels);

    for (int c = 0; c < channels_count; ++c) {
        std::memcpy(output.data() + c * channel_area,
                    channels[static_cast<size_t>(c)].data,
                    static_cast<size_t>(channel_area) * sizeof(float));
    }
}
