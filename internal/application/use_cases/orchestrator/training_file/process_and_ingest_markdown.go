package trainingfile

import (
	"errors"
	"strings"
)

func parseLineBasedChunks(content string) []pipelineChunk {
	lines := strings.Split(content, "\n")
	tokens := make([]lineToken, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		imageMatches := inlineImagePattern.FindAllStringSubmatch(trimmed, -1)
		if len(imageMatches) > 0 {
			for _, m := range imageMatches {
				if len(m) == 2 {
					tokens = append(tokens, lineToken{Type: lineTokenImage, Value: strings.TrimSpace(m[1])})
				}
			}
			trimmed = inlineImagePattern.ReplaceAllString(trimmed, "")
			trimmed = strings.TrimSpace(trimmed)
		}

		if trimmed != "" {
			for _, sentence := range splitSentences(trimmed) {
				s := strings.TrimSpace(sentence)
				if s != "" {
					tokens = append(tokens, lineToken{Type: lineTokenText, Value: s})
				}
			}
		}
	}

	chunks := make([]pipelineChunk, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token.Type == lineTokenText {
			chunk := pipelineChunk{
				Text:       token.Value,
				ImagePaths: []string{},
			}
			for i+1 < len(tokens) && tokens[i+1].Type == lineTokenImage {
				chunk.ImagePaths = append(chunk.ImagePaths, tokens[i+1].Value)
				i++
			}
			chunks = append(chunks, chunk)
			continue
		}

		if len(chunks) > 0 {
			chunks[len(chunks)-1].ImagePaths = append(chunks[len(chunks)-1].ImagePaths, token.Value)
			continue
		}
		chunks = append(chunks, pipelineChunk{
			Text:       "",
			ImagePaths: []string{token.Value},
		})
	}

	out := make([]pipelineChunk, 0, len(chunks))
	for _, c := range chunks {
		if strings.TrimSpace(c.Text) == "" {
			continue
		}
		out = append(out, c)
	}
	return out
}

func splitSentences(input string) []string {
	text := strings.TrimSpace(input)
	if text == "" {
		return nil
	}

	matches := sentenceSplitPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return []string{text}
	}

	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		s := strings.TrimSpace(m[1])
		if s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return []string{text}
	}
	return out
}

func mergeChunksBySemantic(chunks []pipelineChunk, embeddings [][]float32, threshold float32) ([]pipelineChunk, [][]float32, error) {
	if len(chunks) == 0 || len(embeddings) == 0 || len(chunks) != len(embeddings) {
		return nil, nil, errors.New("invalid input for semantic merge")
	}

	mergedChunks := make([]pipelineChunk, 0, len(chunks))
	mergedVectors := make([][]float32, 0, len(chunks))

	currentChunk := chunks[0]
	currentVectors := [][]float32{embeddings[0]}

	for i := 1; i < len(chunks); i++ {
		sim, err := consineSimilarity(embeddings[i-1], embeddings[i])
		if err != nil {
			return nil, nil, err
		}

		if sim >= threshold {
			currentChunk.Text = strings.TrimSpace(currentChunk.Text + " " + chunks[i].Text)
			currentChunk.ImagePaths = append(currentChunk.ImagePaths, chunks[i].ImagePaths...)
			currentVectors = append(currentVectors, embeddings[i])
			continue
		}

		mergedChunks = append(mergedChunks, currentChunk)
		mergedVectors = append(mergedVectors, averageVectors(currentVectors))

		currentChunk = chunks[i]
		currentVectors = [][]float32{embeddings[i]}
	}

	mergedChunks = append(mergedChunks, currentChunk)
	mergedVectors = append(mergedVectors, averageVectors(currentVectors))
	return mergedChunks, mergedVectors, nil
}

func averageVectors(vectors [][]float32) []float32 {
	if len(vectors) == 0 {
		return nil
	}
	dim := len(vectors[0])
	out := make([]float32, dim)
	for _, v := range vectors {
		for i := 0; i < dim && i < len(v); i++ {
			out[i] += v[i]
		}
	}
	for i := range out {
		out[i] = out[i] / float32(len(vectors))
	}
	return out
}
