#include <chrono>
#include <cmath>
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
        std::cout << "[1/5] Config: " << configPath << std::endl;

        std::cout << "[2/5] Initializing services...\n";
        bootstrap::DIContainer container;
        bootstrap::ServiceRegistry::registerServices(container, configPath);

        auto textEncoder   = container.resolve<JinaTextEncoder>();
        auto visionEncoder = container.resolve<JinaVisionEncoder>();

        // ── Prepare test data ──
        std::vector<std::string> texts = {
            "a cat sitting on a couch",
            "a dog running in the park",
            "the sky is blue today",
            "machine learning is transforming the world",
            "neural networks process information in layers",
            "deep learning requires large datasets",
            "transformers use self-attention mechanisms",
            "embedding vectors capture semantic meaning"
        };

        std::vector<cv::Mat> images;
        for (int i = 0; i < 8; ++i) {
            cv::Mat synth(224, 224, CV_8UC3);
            cv::randu(synth, cv::Scalar(0, 0, 0), cv::Scalar(255, 255, 255));
            images.push_back(synth);
        }

        // ══════════════════════════════════════════════════════════
        // [3/5] TEXT BENCHMARK: Single-loop vs Batch
        // ══════════════════════════════════════════════════════════
        std::cout << "\n" << std::string(60, '=') << "\n";
        std::cout << "[3/5] TEXT ENCODER BENCHMARK\n";
        std::cout << std::string(60, '=') << "\n";

        for (int batchSize : {1, 2, 4, 8}) {
            std::vector<std::string> subset(texts.begin(), texts.begin() + batchSize);

            // --- Method A: Single encode in loop ---
            auto t0 = std::chrono::high_resolution_clock::now();
            std::vector<EmbeddingResult> singleResults;
            for (const auto& t : subset) {
                singleResults.push_back(textEncoder->encode(t));
            }
            auto t1 = std::chrono::high_resolution_clock::now();
            auto singleMs = std::chrono::duration<double, std::milli>(t1 - t0).count();

            // --- Method B: Batch encode ---
            auto t2 = std::chrono::high_resolution_clock::now();
            auto batchResults = textEncoder->encodeBatch(subset);
            auto t3 = std::chrono::high_resolution_clock::now();
            auto batchMs = std::chrono::duration<double, std::milli>(t3 - t2).count();

            double speedup = singleMs / batchMs;

            std::cout << "\n  Batch size = " << batchSize << ":\n";
            std::cout << "    Single-loop : " << singleMs << " ms"
                      << "  (" << singleMs / batchSize << " ms/item)\n";
            std::cout << "    Batch       : " << batchMs << " ms"
                      << "  (" << batchMs / batchSize << " ms/item)\n";
            std::cout << "    Speedup     : " << speedup << "x\n";
        }

        // ══════════════════════════════════════════════════════════
        // [4/5] VISION BENCHMARK: Single-loop vs Batch
        // ══════════════════════════════════════════════════════════
        std::cout << "\n" << std::string(60, '=') << "\n";
        std::cout << "[4/5] VISION ENCODER BENCHMARK\n";
        std::cout << std::string(60, '=') << "\n";

        for (int batchSize : {1, 2, 4, 8}) {
            std::vector<cv::Mat> subset(images.begin(), images.begin() + batchSize);

            // --- Method A: Single encode in loop ---
            auto t0 = std::chrono::high_resolution_clock::now();
            std::vector<EmbeddingResult> singleResults;
            for (const auto& img : subset) {
                singleResults.push_back(visionEncoder->encodeFromMat(img));
            }
            auto t1 = std::chrono::high_resolution_clock::now();
            auto singleMs = std::chrono::duration<double, std::milli>(t1 - t0).count();

            // --- Method B: Batch encode ---
            auto t2 = std::chrono::high_resolution_clock::now();
            auto batchResults = visionEncoder->encodeBatchFromMat(subset);
            auto t3 = std::chrono::high_resolution_clock::now();
            auto batchMs = std::chrono::duration<double, std::milli>(t3 - t2).count();

            double speedup = singleMs / batchMs;

            std::cout << "\n  Batch size = " << batchSize << ":\n";
            std::cout << "    Single-loop : " << singleMs << " ms"
                      << "  (" << singleMs / batchSize << " ms/item)\n";
            std::cout << "    Batch       : " << batchMs << " ms"
                      << "  (" << batchMs / batchSize << " ms/item)\n";
            std::cout << "    Speedup     : " << speedup << "x\n";
        }

        // ══════════════════════════════════════════════════════════
        // [5/5] RESULT VERIFICATION
        // ══════════════════════════════════════════════════════════
        std::cout << "\n" << std::string(60, '=') << "\n";
        std::cout << "[5/5] RESULT VERIFICATION\n";
        std::cout << std::string(60, '=') << "\n";

        auto singleEmb = textEncoder->encode(texts[0]);
        auto batchEmb  = textEncoder->encodeBatch({texts[0]});
        float diff = 0.0f;
        for (size_t i = 0; i < static_cast<size_t>(singleEmb.dimension()); ++i) {
            float d = singleEmb.data()[i] - batchEmb[0].data()[i];
            diff += d * d;
        }
        diff = std::sqrt(diff);
        std::cout << "\n  Single vs Batch[0] L2 distance: " << diff;
        std::cout << (diff < 1e-5f ? "  MATCH" : "  MISMATCH") << "\n";

        printEmbedding("\n  Single", singleEmb);
        printEmbedding("  Batch[0]", batchEmb[0]);

        std::cout << "\n=== Done ===\n";
        return 0;
    } catch (const std::exception& e) {
        std::cerr << "Error: " << e.what() << std::endl;
        return 1;
    }
}
