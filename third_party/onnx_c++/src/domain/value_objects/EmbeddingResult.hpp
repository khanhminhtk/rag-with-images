#ifndef ONNX_EMBEDDING_RESULT_HPP
#define ONNX_EMBEDDING_RESULT_HPP

#include <cmath>
#include <stdexcept>
#include <string>
#include <vector>

class EmbeddingResult {
public:
    EmbeddingResult() = default;

    EmbeddingResult(std::vector<float> data, const int dimension)
        : data_(std::move(data)), dimension_(dimension) {
        if (dimension_ <= 0) {
            throw std::invalid_argument("Embedding dimension must be positive");
        }
        if (static_cast<int>(data_.size()) != dimension_) {
            throw std::invalid_argument(
                "Data size (" + std::to_string(data_.size()) +
                ") does not match dimension (" + std::to_string(dimension_) + ")");
        }
    }

    [[nodiscard]] const std::vector<float>& data() const { return data_; }
    [[nodiscard]] int dimension() const { return dimension_; }
    [[nodiscard]] bool empty() const { return data_.empty(); }

    [[nodiscard]] float norm() const {
        float sum_sq = 0.0f;
        for (float v : data_) sum_sq += v * v;
        return std::sqrt(sum_sq);
    }

    [[nodiscard]] EmbeddingResult normalized() const {
        const float n = norm();
        if (n < 1e-12f) return *this;
        std::vector<float> normed(data_.size());
        for (size_t i = 0; i < data_.size(); ++i) {
            normed[i] = data_[i] / n;
        }
        return EmbeddingResult(std::move(normed), dimension_);
    }

    [[nodiscard]] static float cosineSimilarity(const EmbeddingResult& a,
                                                 const EmbeddingResult& b) {
        if (a.dimension() != b.dimension()) {
            throw std::invalid_argument("Cosine similarity: dimension mismatch");
        }
        float dot = 0.0f, norm_a = 0.0f, norm_b = 0.0f;
        for (size_t i = 0; i < static_cast<size_t>(a.dimension()); ++i) {
            dot    += a.data_[i] * b.data_[i];
            norm_a += a.data_[i] * a.data_[i];
            norm_b += b.data_[i] * b.data_[i];
        }
        const float denom = std::sqrt(norm_a) * std::sqrt(norm_b);
        if (denom < 1e-12f) return 0.0f;
        return dot / denom;
    }

private:
    std::vector<float> data_;
    int dimension_ = 0;
};

#endif
