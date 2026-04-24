package ports



type InferenceResponse struct {
	Embedding []float32
	Dimension int
}

type Inference interface {
	EmbedText(text string) ([]float32, error)
	EmbedImage(pixels []byte, width, height, channels int) ([]float32, error)
	EmbedBatchText(texts []string) ([][]float32, error)
	EmbedBatchImage(images [][]byte, width, height, channels int) ([][]float32, error)
	Close()
}
