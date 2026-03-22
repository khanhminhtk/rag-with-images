# mRAG for Slide/PDF Documents

Project này xây dựng hệ thống **multimodal RAG (mRAG)** cho truy vấn tài liệu công khai như:
- Slide bài giảng
- PDF kỹ thuật
- Tài liệu sản phẩm/manual

Mục tiêu là hỏi đáp theo ngữ cảnh tài liệu, tận dụng cả text OCR và thông tin hình ảnh/layout.

## Bài toán

Tài liệu PDF/slide thường chứa:
- Văn bản dài, chia section
- Hình minh họa, biểu đồ, bảng
- Nội dung khó tìm bằng keyword thuần

Project tập trung vào việc biểu diễn các đơn vị nội dung dưới dạng vector + metadata để truy vấn chính xác theo ngữ nghĩa.

## Phạm vi hệ thống

- Trích xuất text/OCR và thông tin vùng nội dung (page, bbox, section)
- Sinh embedding cho text và image
- Lưu trữ point (vector + payload) trong Qdrant
- Truy vấn semantic search theo câu hỏi người dùng
- Gọi LLM cloud để tổng hợp câu trả lời có bám ngữ cảnh

## Tech Stack

- **Backend**: Go
- **API service**: gRPC, RestAPI
- **Vector Database**: Qdrant
- **LLM Cloud**: Gemini/OpenAI-compatible
- **Embedding Inference**: C++ + ONNX Runtime

## Dữ liệu demo phù hợp

Project ưu tiên nguồn dữ liệu mở, dễ lấy và dễ chia sẻ:
- Lecture slide decks
- Documentation PDF từ open-source projects
- Public technical reports/whitepapers

Hướng dữ liệu này giúp demo nhanh, ít rủi ro dữ liệu nhạy cảm, và dễ mở rộng tập test.
