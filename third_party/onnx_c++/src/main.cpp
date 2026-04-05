#include <exception>
#include <filesystem>
#include <iostream>
#include <string>

#include <opencv2/opencv.hpp>

#include "bootstrap/container.hpp"
#include "bootstrap/registry.hpp"
#include "domain/value_objects/EmbeddingResult.hpp"
#include "infra/jina_text_encoder.hpp"
#include "infra/jina_vision_encoder.hpp"

namespace {

void printEmbedding(const std::string& label, const EmbeddingResult& emb, int showN = 5) {
    std::cout << label << " [dim=" << emb.dimension() << ", norm=" << emb.norm() << "]:\n  [";
    for (int i = 0; i < std::min(showN, emb.dimension()); ++i) {
        if (i > 0) std::cout << ", ";
        std::cout << emb.data()[static_cast<size_t>(i)];
    }
    std::cout << ", ...]\n";
}

std::string resolveConfigPath(int argc, char* argv[]) {
    if (argc > 1) return argv[1];
    for (const auto& candidate : {"config/config.yaml", "../config/config.yaml"}) {
        if (std::filesystem::exists(candidate)) return candidate;
    }
    return "config/config.yaml";
}

}  // namespace

int main(int argc, char* argv[]) {
    try {
        const std::string configPath = resolveConfigPath(argc, argv);
        std::cout << "[1/4] Config: " << configPath << std::endl;

        std::cout << "[2/4] Initializing services...\n";
        bootstrap::DIContainer container;
        bootstrap::ServiceRegistry::registerServices(container, configPath);

        std::cout << "\n[3/4] Text Embedding\n";
        auto textEncoder = container.resolve<JinaTextEncoder>();
        std::string demoText = "a cat sitting on a couch";
        auto textEmb = textEncoder->encode(demoText);
        printEmbedding("Text", textEmb);

        std::cout << "\n[4/4] Vision Embedding\n";
        auto visionEncoder = container.resolve<JinaVisionEncoder>();

        if (argc > 2) {
            cv::Mat image = cv::imread(argv[2]);
            if (image.empty()) {
                std::cerr << "Failed to load image: " << argv[2] << std::endl;
                return 1;
            }
            auto visEmb = visionEncoder->encodeFromMat(image);
            printEmbedding("Vision", visEmb);
            std::cout << "\nCosine similarity = "
                      << EmbeddingResult::cosineSimilarity(textEmb, visEmb) << "\n";
        } else {
            cv::Mat synth(224, 224, CV_8UC3);
            cv::randu(synth, cv::Scalar(0, 0, 0), cv::Scalar(255, 255, 255));
            auto visEmb = visionEncoder->encodeFromMat(synth);
            printEmbedding("Vision (synthetic)", visEmb);
            std::cout << "\nCosine similarity = "
                      << EmbeddingResult::cosineSimilarity(textEmb, visEmb) << "\n";
        }

        std::cout << "\n=== Done ===\n";
        return 0;
    } catch (const std::exception& e) {
        std::cerr << "Error: " << e.what() << std::endl;
        return 1;
    }
}
