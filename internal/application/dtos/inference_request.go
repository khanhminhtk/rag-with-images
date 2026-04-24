package dtos

type EmbedTextRequest struct {
	Text string `json:"text" validate:"required"`
}

type EmbedTextResponse struct {
	Embedding []float32 `json:"embedding"`
	Dimension int       `json:"dimension"`
	Status    bool      `json:"status"`
}

type EmbedBatchTextRequest struct {
	Texts []string `json:"texts" validate:"required"`
}

type EmbedBatchTextResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Dimension  int         `json:"dimension"`
	Status     bool        `json:"status"`
}

type EmbedImageRequest struct {
	Pixels   []byte `json:"pixels" validate:"required"`
	Width    int    `json:"width" validate:"required"`
	Height   int    `json:"height" validate:"required"`
	Channels int    `json:"channels" validate:"required"`
}

type EmbedImageResponse struct {
	Embedding []float32 `json:"embedding"`
	Dimension int       `json:"dimension"`
	Status    bool      `json:"status"`
}

type EmbedBatchImageRequest struct {
	Images   [][]byte `json:"images" validate:"required"`
	Width    int      `json:"width" validate:"required"`
	Height   int      `json:"height" validate:"required"`
	Channels int      `json:"channels" validate:"required"`
}

type EmbedBatchImageResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Dimension  int         `json:"dimension"`
	Status     bool        `json:"status"`
}
